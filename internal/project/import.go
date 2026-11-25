package project

import (
	"context"
	"fmt"
	"path/filepath"
)

const MaxImportFiles = 64

type ImportFailure struct {
	Path string
	Error string
}

type ImportBatch struct {
	Project Project
	Selected int
	Added,Duplicates int
	Failures []ImportFailure
}

func (p Project) ImportPaths(ctx context.Context,paths []string,inspect func(context.Context,string)(Asset,error)) (ImportBatch,error) {
	b:=ImportBatch{Project:p,Selected:-1}
	if len(paths)==0 || len(paths)>MaxImportFiles {return b,fmt.Errorf("select between 1 and %d files",MaxImportFiles)}
	for _,path:=range paths {
		if err:=ctx.Err();err!=nil {return ImportBatch{},err}
		abs,err:=filepath.Abs(path)
		if err!=nil {
			b.Failures=append(b.Failures,ImportFailure{Path:path,Error:err.Error()})
			continue
		}
		if i,ok:=b.Project.FindAssetPath(abs);ok {
			b.Selected=i
			b.Duplicates++
			continue
		}
		a,err:=inspect(ctx,abs)
		if ctx.Err()!=nil {return ImportBatch{},ctx.Err()}
		if err==nil {
			var q Project
			var i int
			var added bool
			q,i,added,err=b.Project.AddAsset(a)
			if err==nil {
				b.Project=q
				b.Selected=i
				if added {b.Added++} else {b.Duplicates++}
				continue
			}
		}
		b.Failures=append(b.Failures,ImportFailure{Path:abs,Error:err.Error()})
	}
	return b,nil
}
