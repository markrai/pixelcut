package projectexport

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

type matrixProcessKey struct {
	pid     uint32
	created int64
}

type matrixProcessSample struct {
	working, private, cpu uint64
}

type matrixHostSample struct {
	at      time.Time
	root    matrixProcessKey
	procs   map[matrixProcessKey]matrixProcessSample
	missing int
}

var matrixHostSnapshot = func() (matrixHostSample, error) {
	return matrixHostSample{}, errors.New("Windows process-tree sampling unavailable")
}

type matrixGPUSample struct {
	Index                                               int
	UUID, Name, Driver                                  string
	UsedMiB, GPUPercent, DecoderPercent, EncoderPercent uint64
}

func parseMatrixGPU(raw string) ([]matrixGPUSample, error) {
	r := csv.NewReader(strings.NewReader(raw))
	r.FieldsPerRecord = 8
	var result []matrixGPUSample
	seen := make(map[string]bool)
	indexes := make(map[int]bool)
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		for i := range row {
			row[i] = strings.TrimSpace(row[i])
		}
		index, err := strconv.Atoi(row[0])
		if err != nil || index < 0 || row[1] == "" || row[2] == "" || row[3] == "" || seen[row[1]] || indexes[index] || len(result) >= 32 {
			return nil, errors.New("invalid or duplicate NVIDIA device identity")
		}
		var nums [4]uint64
		for i := range nums {
			nums[i], err = strconv.ParseUint(row[i+4], 10, 64)
			if err != nil || (i > 0 && nums[i] > 100) {
				return nil, fmt.Errorf("unavailable or invalid NVIDIA metric %q on device %s", row[i+4], row[1])
			}
		}
		seen[row[1]], indexes[index] = true, true
		result = append(result, matrixGPUSample{index, row[1], row[2], row[3], nums[0], nums[1], nums[2], nums[3]})
	}
	if len(result) == 0 {
		return nil, errors.New("no NVIDIA device samples")
	}
	slices.SortFunc(result, func(a, b matrixGPUSample) int { return strings.Compare(a.UUID, b.UUID) })
	return result, nil
}

type matrixResourceOutput struct {
	data []byte
}

func (b *matrixResourceOutput) Write(p []byte) (int, error) {
	if len(p) > 65536-len(b.data) {
		return 0, errors.New("NVIDIA resource output exceeded 64 KiB")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func matrixGPUQuery(ctx context.Context, exe string) ([]matrixGPUSample, error) {
	job, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(job, exe, "--query-gpu=index,uuid,name,driver_version,memory.used,utilization.gpu,utilization.decoder,utilization.encoder", "--format=csv,noheader,nounits")
	var stdout matrixResourceOutput
	var stderr errorTail
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		if job.Err() != nil {
			return nil, job.Err()
		}
		return nil, fmt.Errorf("nvidia-smi: %w: %s", err, stderr.data)
	}
	return parseMatrixGPU(string(stdout.data))
}

type matrixGPUResource struct {
	Baseline                                                            matrixGPUSample
	PeakUsedMiB, PeakGPUPercent, PeakDecoderPercent, PeakEncoderPercent uint64
	sumGPU, sumDecode, sumEncode                                        uint64
	MeanGPUPercent, MeanDecoderPercent, MeanEncoderPercent              float64
}

type matrixResourceSummary struct {
	HostSamples, GPUSamples, MissingProcessObservations, PeakTreeProcesses            int
	PeakRootWorkingBytes, PeakRootPrivateCommitBytes                                  uint64
	PeakChildrenWorkingBytes, PeakChildrenPrivateCommitBytes                          uint64
	PeakTreeWorkingBytes, PeakTreePrivateCommitBytes                                  uint64
	ObservedCPUSecondsLowerBound, CPUSampleWindowSeconds, ObservedMeanCoresLowerBound float64
	DeviceWideGPU                                                                     []matrixGPUResource
}

func (s *matrixResourceSummary) addHost(previous, current matrixHostSample) error {
	root, ok := current.procs[current.root]
	if !ok || current.root != previous.root || !current.at.After(previous.at) {
		return errors.New("invalid process-tree sample identity or time")
	}
	var working, private, ticks uint64
	for key, p := range current.procs {
		if key != current.root {
			working += p.working
			private += p.private
		}
		if old, ok := previous.procs[key]; ok {
			if p.cpu < old.cpu {
				return errors.New("process CPU counter regressed")
			}
			ticks += p.cpu - old.cpu
		} else if key.created >= previous.at.UnixNano() && key.created <= current.at.UnixNano() {
			ticks += p.cpu
		}
	}
	s.HostSamples++
	s.MissingProcessObservations += current.missing
	s.PeakTreeProcesses = max(s.PeakTreeProcesses, len(current.procs))
	s.PeakRootWorkingBytes = max(s.PeakRootWorkingBytes, root.working)
	s.PeakRootPrivateCommitBytes = max(s.PeakRootPrivateCommitBytes, root.private)
	s.PeakChildrenWorkingBytes = max(s.PeakChildrenWorkingBytes, working)
	s.PeakChildrenPrivateCommitBytes = max(s.PeakChildrenPrivateCommitBytes, private)
	s.PeakTreeWorkingBytes = max(s.PeakTreeWorkingBytes, working+root.working)
	s.PeakTreePrivateCommitBytes = max(s.PeakTreePrivateCommitBytes, private+root.private)
	s.ObservedCPUSecondsLowerBound += float64(ticks) / 1e7
	s.CPUSampleWindowSeconds += current.at.Sub(previous.at).Seconds()
	s.ObservedMeanCoresLowerBound = s.ObservedCPUSecondsLowerBound / s.CPUSampleWindowSeconds
	return nil
}

func (s *matrixResourceSummary) addGPU(samples []matrixGPUSample) error {
	if len(samples) != len(s.DeviceWideGPU) {
		return errors.New("NVIDIA device count changed")
	}
	for i, p := range samples {
		b := s.DeviceWideGPU[i].Baseline
		if p.Index != b.Index || p.UUID != b.UUID || p.Name != b.Name || p.Driver != b.Driver {
			return errors.New("NVIDIA device identity changed")
		}
	}
	s.GPUSamples++
	for i, p := range samples {
		g := &s.DeviceWideGPU[i]
		g.PeakUsedMiB = max(g.PeakUsedMiB, p.UsedMiB)
		g.PeakGPUPercent = max(g.PeakGPUPercent, p.GPUPercent)
		g.PeakDecoderPercent = max(g.PeakDecoderPercent, p.DecoderPercent)
		g.PeakEncoderPercent = max(g.PeakEncoderPercent, p.EncoderPercent)
		g.sumGPU += p.GPUPercent
		g.sumDecode += p.DecoderPercent
		g.sumEncode += p.EncoderPercent
		g.MeanGPUPercent = float64(g.sumGPU) / float64(s.GPUSamples)
		g.MeanDecoderPercent = float64(g.sumDecode) / float64(s.GPUSamples)
		g.MeanEncoderPercent = float64(g.sumEncode) / float64(s.GPUSamples)
	}
	return nil
}

type matrixResourceSession struct {
	cancel  context.CancelFunc
	done    chan struct{}
	summary matrixResourceSummary
	err     error
}

func startMatrixResources(ctx context.Context) (*matrixResourceSession, error) {
	exe, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return nil, err
	}
	gpu, err := matrixGPUQuery(ctx, exe)
	if err != nil {
		return nil, err
	}
	host, err := matrixHostSnapshot()
	if err != nil {
		return nil, err
	}
	job, cancel := context.WithCancel(ctx)
	s := &matrixResourceSession{cancel: cancel, done: make(chan struct{})}
	for _, g := range gpu {
		s.summary.DeviceWideGPU = append(s.summary.DeviceWideGPU, matrixGPUResource{Baseline: g})
	}
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-job.Done():
				return
			case <-ticker.C:
				next, err := matrixHostSnapshot()
				if err == nil {
					err = s.summary.addHost(host, next)
				}
				if err != nil {
					s.err = err
					return
				}
				host = next
				gpu, err := matrixGPUQuery(job, exe)
				if job.Err() != nil {
					return
				}
				if err == nil {
					err = s.summary.addGPU(gpu)
				}
				if err != nil {
					s.err = err
					return
				}
			}
		}
	}()
	return s, nil
}

func (s *matrixResourceSession) stop() (string, error) {
	s.cancel()
	<-s.done
	err := s.err
	if err == nil && (s.summary.HostSamples < 2 || s.summary.GPUSamples < 2 || s.summary.PeakTreeProcesses < 2) {
		err = errors.New("insufficient process-tree or GPU samples")
	}
	b, marshalErr := json.Marshal(s.summary)
	return string(b), errors.Join(err, marshalErr)
}

func TestMatrixGPUResourceParser(t *testing.T) {
	raw := "1, GPU-b, NVIDIA Test B, 123.45, 400, 10, 20, 30\n0, GPU-a, NVIDIA Test A, 123.45, 200, 40, 50, 60\n"
	gpu, err := parseMatrixGPU(raw)
	if err != nil || len(gpu) != 2 || gpu[0].UUID != "GPU-a" || gpu[0].EncoderPercent != 60 || gpu[1].DecoderPercent != 20 {
		t.Fatalf("GPU inventory: %+v %v", gpu, err)
	}
	for _, bad := range []string{"", raw + raw, strings.Replace(raw, "400", "[N/A]", 1), strings.Replace(raw, ", 30", ", 101", 1), strings.Replace(raw, "GPU-b", "GPU-a", 1)} {
		if _, err := parseMatrixGPU(bad); err == nil {
			t.Fatalf("invalid GPU sample accepted: %q", bad)
		}
	}
	s := matrixResourceSummary{DeviceWideGPU: []matrixGPUResource{{Baseline: gpu[0]}, {Baseline: gpu[1]}}}
	if err := s.addGPU(gpu); err != nil {
		t.Fatal(err)
	}
	gpu[0].EncoderPercent = 20
	gpu[0].UsedMiB = 300
	if err := s.addGPU(gpu); err != nil {
		t.Fatal(err)
	}
	if s.GPUSamples != 2 || s.DeviceWideGPU[0].PeakUsedMiB != 300 || s.DeviceWideGPU[0].PeakEncoderPercent != 60 || s.DeviceWideGPU[0].MeanEncoderPercent != 40 {
		t.Fatalf("incorrect GPU aggregate: %+v", s)
	}
	gpu[0].Driver = "changed"
	if err := s.addGPU(gpu); err == nil {
		t.Fatal("changed GPU identity accepted")
	}
}

func TestMatrixHostResourceAggregation(t *testing.T) {
	start := time.Unix(100, 0)
	root := matrixProcessKey{pid: 1, created: start.Add(-time.Hour).UnixNano()}
	child := matrixProcessKey{pid: 2, created: start.Add(time.Millisecond).UnixNano()}
	a := matrixHostSample{at: start, root: root, procs: map[matrixProcessKey]matrixProcessSample{root: {100, 200, 5000000}}}
	b := matrixHostSample{at: start.Add(time.Second), root: root, procs: map[matrixProcessKey]matrixProcessSample{root: {120, 220, 10000000}, child: {300, 400, 10000000}}}
	c := matrixHostSample{at: start.Add(2 * time.Second), root: root, procs: map[matrixProcessKey]matrixProcessSample{root: {500, 600, 15000000}}, missing: 1}
	var s matrixResourceSummary
	if err := s.addHost(a, b); err != nil {
		t.Fatal(err)
	}
	if err := s.addHost(b, c); err != nil {
		t.Fatal(err)
	}
	if s.HostSamples != 2 || s.PeakTreeWorkingBytes != 500 || s.PeakTreePrivateCommitBytes != 620 || s.PeakChildrenWorkingBytes != 300 || s.PeakRootWorkingBytes != 500 || s.ObservedCPUSecondsLowerBound != 2 || s.ObservedMeanCoresLowerBound != 1 || s.MissingProcessObservations != 1 || s.PeakTreeProcesses != 2 {
		t.Fatalf("incorrect resource aggregate: %+v", s)
	}
	if err := s.addHost(c, b); err == nil {
		t.Fatal("nonmonotonic timestamp accepted")
	}
}

func TestMatrixResourceOutputLimit(t *testing.T) {
	var b matrixResourceOutput
	if _, err := b.Write(make([]byte, 65536)); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Write([]byte{1}); err == nil {
		t.Fatal("unbounded query output")
	}
}

