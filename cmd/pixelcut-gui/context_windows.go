package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

const wmContextMenu=0x007b

const (
	menuPasteHere=2001+iota
	menuGapHere
	menuRevealClip
	menuRevealAsset
	menuSelectAsset
	menuRelinkClip
	menuReplaceClip
)

type menuItem struct {
	id int
	label string
	enabled,checked bool
}

func item(id int,label string,enabled bool) menuItem {return menuItem{id:id,label:label,enabled:enabled}}
func separator() menuItem {return menuItem{}}

type timelineMenuState struct {
	count int
	audioTrack bool
	frameSnap bool
	offline bool
	split,paste,pasteHere,gap,hasClips,undo,redo,snap,ripple bool
}

func timelineMenuItems(s timelineMenuState) []menuItem {
	var items []menuItem
	if s.count>0 {
		single:=s.count==1
		items=append(items,item(idSplit,"Split at playhead\tS",single&&s.split),separator(),
			item(idCopy,"Copy\tCtrl+C",true),item(idCut,"Cut\tCtrl+X",true),
			item(idPaste,"Paste at playhead\tCtrl+V",s.paste),item(idDuplicate,"Duplicate\tCtrl+D",true),separator(),
			item(idSetIn,"Set source In at playhead\tI",single&&s.split&&!s.offline&&!s.audioTrack),item(idSetOut,"Set source Out at playhead\tO",single&&s.split&&!s.offline&&!s.audioTrack),separator(),
			item(idCropVisual,"Visual crop",single&&!s.offline&&!s.audioTrack),item(idTransform,"Transform",single&&!s.offline&&!s.audioTrack),item(idExport,"Export clip\tCtrl+E",single&&!s.offline&&!s.audioTrack),separator(),
			item(idDelete,"Delete\tDel",true),item(idRippleDelete,"Ripple delete\tShift+Del",true),separator(),
			item(menuSelectAsset,"Select source in media bin",single),item(menuRevealClip,"Show source in Explorer",single&&!s.offline),
			item(menuRelinkClip,"Relink source...",single&&s.offline),item(menuReplaceClip,"Replace source...",single))
	} else {
		items=append(items,item(menuPasteHere,"Paste here",s.pasteHere),item(idPaste,"Paste at playhead\tCtrl+V",s.paste),
			item(menuGapHere,"Remove gap here",s.gap),separator(),
			item(idSelectAll,"Select all clips\tCtrl+A",s.hasClips),
			item(idUndo,"Undo\tCtrl+Z",s.undo),item(idRedo,"Redo\tCtrl+Y",s.redo))
	}
	return append(items,separator(),menuItem{id:idSnap,label:"Snapping\tN",enabled:true,checked:s.snap},
		menuItem{id:idRippleTrim,label:"Ripple trimming\tR",enabled:true,checked:s.ripple},menuItem{id:idFrameSnap,label:"Source-frame trim snapping\tB",enabled:true,checked:s.frameSnap},item(idFit,"Fit timeline\tF",true))
}

func assetMenuItems(selected,offline,hasMissing bool) []menuItem {
	return []menuItem{item(idImport,"Import videos...\tCtrl+I",true),separator(),
		item(idAdd,"Add to end of active track",selected),item(menuRevealAsset,"Show source in Explorer",selected&&!offline),separator(),
		item(idRelinkAsset,"Relink offline asset...",selected&&offline),item(idReplaceAsset,"Replace source...",selected),
		item(idRelinkFolder,"Search folder for offline assets...",hasMissing),separator(),
		item(idRemoveAsset,"Remove asset\tDel",selected)}
}

func (e *editor) canOpenMenu() bool {
	return !e.menuOpen && !e.busy && e.drag==nil && e.assetDrag==nil && e.marquee==nil && !e.scrubbing && e.cropSession==nil && e.transformSession==nil
}

func contextPoint(hwnd,lp uintptr,keyboard point) (point,point) {
	screen:=point{int32(int16(lp&0xffff)),int32(int16((lp>>16)&0xffff))}
	if screen.X==-1 && screen.Y==-1 {
		local:=keyboard
		screen=local
		user.NewProc("ClientToScreen").Call(hwnd,uintptr(unsafe.Pointer(&screen)))
		return screen,local
	}
	local:=screen
	user.NewProc("ScreenToClient").Call(hwnd,uintptr(unsafe.Pointer(&local)))
	return screen,local
}

func (e *editor) popup(items []menuItem,screen point) int {
	menu,_,_:=user.NewProc("CreatePopupMenu").Call()
	if menu==0 {e.tell("Unable to create context menu.");return 0}
	defer user.NewProc("DestroyMenu").Call(menu)
	for _,v:=range items {
		flags:=uintptr(0)
		var label *uint16
		if v.id==0 {flags=0x800} else {
			if !v.enabled {flags|=0x3}
			if v.checked {flags|=0x8}
			label=u(v.label)
		}
		ok,_,_:=user.NewProc("AppendMenuW").Call(menu,flags,uintptr(v.id),uintptr(unsafe.Pointer(label)))
		runtime.KeepAlive(label)
		if ok==0 {e.tell("Unable to build context menu.");return 0}
	}
	e.menuOpen=true
	id,_,_:=user.NewProc("TrackPopupMenuEx").Call(menu,0x0100|0x0080|0x0002,uintptr(screen.X),uintptr(screen.Y),mainWindow,0)
	e.menuOpen=false
	for _,v:=range items {if v.id!=0 && v.id==int(id) && v.enabled {return int(id)}}
	return 0
}

func timelineMenuHit(x,y,left,right int32,hasAudio bool) bool {
	return x>=trackHeader && x>=left && x<right && ((y>=laneTop && y<laneBottom) || (hasAudio && y>=104 && y<140))
}

func (e *editor) pastePossible(at int64) bool {
	if e.clipboard.Count()==0 {return false}
	ids:=make([]string,e.clipboard.Count())
	prefix:=nextID("menu-check")
	for i:=range ids {ids[i]=fmt.Sprintf("%s-%d",prefix,i)}
	_,err:=e.p.PasteClips(e.trackID(),e.clipboard,at,ids)
	return err==nil
}

func (e *editor) timelineContext(lp uintptr) {
	if !e.canOpenMenu() {return}
	e.checkSourceVersions(true)
	if e.mediaListDirty {e.refreshMediaList()}
	w:=clientRect(e.timeline).Right
	keyboard:=lp==^uintptr(0) || uint32(lp)==0xffffffff
	pos:=point{max(trackHeader,min(w-1,e.xAt(e.playhead,w))),laneTop+8}
	if keyboard {
		if c,ok:=e.selected();ok {
			l,r:=max(trackHeader,e.xAt(c.Start,w)),min(w,e.xAt(c.Start+(c.Out-c.In),w))
			if r>l {pos.X=l+(r-l)/2}
		}
	}
	screen,local:=contextPoint(e.timeline,lp,pos)
	if local.X<trackHeader || local.X>=w {return}
	user.NewProc("SetFocus").Call(e.timeline)
	e.updatePlaybackClock()
	if e.playing() {e.pausePreview()}
	hit:=-1
	if keyboard {
		if _,ok:=e.selected();ok {hit=e.clip}
	} else {
		for i,c:=range e.track().Clips {
			a,_:=e.p.Asset(c.AssetID)
			if timelineMenuHit(local.X,local.Y,e.xAt(c.Start,w),e.xAt(c.Start+(c.Out-c.In),w),a.HasAudio && !e.audioTrack()) {hit=i;break}
		}
	}
	at:=e.timeAt(local.X,w)
	if keyboard {at=e.playhead}
	s:=timelineMenuState{audioTrack:e.audioTrack(),paste:e.pastePossible(e.playhead),hasClips:len(e.track().Clips)>0,undo:len(e.undo)>0,redo:len(e.redo)>0,snap:!e.snapDisabled,ripple:e.rippleTrim,frameSnap:e.frameSnap}
	if hit>=0 {
		if !e.clipSelected(hit,e.track().Clips[hit]) {e.selectClip(hit)}
		s.count=len(e.selectionIDs())
		if c,ok:=e.selected();ok {
			s.split=e.playhead>c.Start && e.playhead<c.Start+(c.Out-c.In)
			if a,ok:=e.p.Asset(c.AssetID);ok {s.offline=e.offline(a.Path)}
		}
		e.checkpoint()
	} else {
		s.pasteHere=e.pastePossible(at)
		_,_,err:=e.p.RemoveGap(e.trackID(),at)
		s.gap=err==nil
	}
	id:=e.popup(timelineMenuItems(s),screen)
	if id==0 {return}
	var err error
	switch id {
	case menuPasteHere:
		err=e.pasteAt(false,&at)
	case menuGapHere:
		err=e.removeGapAt(at)
	case menuRelinkClip,menuReplaceClip:
		if c,ok:=e.selected();ok && len(e.selectionIDs())==1 {err=e.relinkOne(c.AssetID,id==menuReplaceClip)}
	case menuSelectAsset,menuRevealClip:
		if c,ok:=e.selected();ok && len(e.selectionIDs())==1 {
			if id==menuRevealClip {
				if a,ok:=e.p.Asset(c.AssetID);ok {err=revealSource(a.Path)}
			} else {
				for i,a:=range e.p.Assets {
					if a.ID==c.AssetID {
						e.asset=i
						send.Call(e.controls[idAssets],lbSet,uintptr(i),0)
						e.checkpoint()
						e.tell("Source selected in media bin.")
						user.NewProc("SetFocus").Call(e.controls[idAssets])
						break
					}
				}
			}
		}
	default:
		if id==idSplit {change(e.controls[idAt],fmtTime(e.playhead))}
		err=e.command(id,0)
	}
	if err!=nil {e.tell(err.Error())}
}

func (e *editor) mediaContext(hwnd,lp uintptr) {
	if !e.canOpenMenu() {return}
	e.checkSourceVersions(true)
	if e.mediaListDirty {e.refreshMediaList()}
	keyboard:=uint32(lp)==0xffffffff
	pos:=point{8,8}
	if keyboard && e.asset>=0 && e.asset<len(e.p.Assets) {
		var r rect
		ok,_,_:=send.Call(hwnd,0x0198,uintptr(e.asset),uintptr(unsafe.Pointer(&r)))
		if int32(ok)>=0 && r.Bottom>0 && r.Top<clientRect(hwnd).Bottom {pos.Y=max(0,r.Top)+min(8,max(0,r.Bottom-r.Top-1))}
	}
	screen,local:=contextPoint(hwnd,lp,pos)
	user.NewProc("SetFocus").Call(hwnd)
	index:=-1
	if keyboard {
		if e.asset>=0 && e.asset<len(e.p.Assets) {index=e.asset}
	} else {
		packed:=uintptr(uint16(local.X))|uintptr(uint16(local.Y))<<16
		r,_,_:=send.Call(hwnd,0x01a9,0,packed)
		i:=int(r&0xffff)
		if (r>>16)&0xffff==0 && i<len(e.p.Assets) {
			var bounds rect
			ok,_,_:=send.Call(hwnd,0x0198,uintptr(i),uintptr(unsafe.Pointer(&bounds)))
			if int32(ok)>=0 && local.Y>=bounds.Top && local.Y<bounds.Bottom {index=i}
		}
	}
	if index>=0 {
		e.asset=index
		send.Call(hwnd,lbSet,uintptr(index),0)
		e.checkpoint()
	}
	offline:=index>=0 && e.offline(e.p.Assets[index].Path)
	id:=e.popup(assetMenuItems(index>=0,offline,e.missingCount()>0),screen)
	if id==0 {return}
	var err error
	if id==menuRevealAsset {err=revealSource(e.p.Assets[index].Path)} else {err=e.command(id,0)}
	if err!=nil {e.tell(err.Error())}
}

func revealSource(path string) error {
	if !filepath.IsAbs(path) {return fmt.Errorf("invalid source path")}
	info,err:=os.Stat(path)
	if err!=nil {return fmt.Errorf("source unavailable: %w",err)}
	if !info.Mode().IsRegular() {return fmt.Errorf("source is not a regular file")}
	ole:=syscall.NewLazyDLL("ole32.dll")
	hr,_,_:=ole.NewProc("CoInitializeEx").Call(0,2)
	if int32(hr)<0 && uint32(hr)!=0x80010106 {return fmt.Errorf("Explorer COM initialization failed: 0x%x",uint32(hr))}
	if int32(hr)>=0 {defer ole.NewProc("CoUninitialize").Call()}
	var pidl uintptr
	name:=u(path)
	hr,_,_ = shell.NewProc("SHParseDisplayName").Call(uintptr(unsafe.Pointer(name)),0,uintptr(unsafe.Pointer(&pidl)),0,0)
	runtime.KeepAlive(name)
	if pidl!=0 {defer ole.NewProc("CoTaskMemFree").Call(pidl)}
	if int32(hr)<0 || pidl==0 {return fmt.Errorf("could not resolve source in Explorer: 0x%x",uint32(hr))}
	hr,_,_=shell.NewProc("SHOpenFolderAndSelectItems").Call(pidl,0,0,0)
	if int32(hr)<0 {return fmt.Errorf("Explorer selection failed: 0x%x",uint32(hr))}
	return nil
}
