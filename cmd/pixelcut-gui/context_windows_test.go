package main

import "testing"

func findMenuItem(items []menuItem,id int) (menuItem,bool) {
	for _,v:=range items {if v.id==id {return v,true}}
	return menuItem{},false
}

func TestTimelineContextEligibility(t *testing.T) {
	for _,tc:=range []struct{
		state timelineMenuState
		id int
		exists,enabled bool
	}{
		{timelineMenuState{count:1,split:true},idSplit,true,true},
		{timelineMenuState{count:1},idSplit,true,false},
		{timelineMenuState{count:2,split:true},idSplit,true,false},
		{timelineMenuState{count:2},idCopy,true,true},
		{timelineMenuState{count:2},idRippleDelete,true,true},
		{timelineMenuState{count:2},idCropVisual,true,false},
		{timelineMenuState{count:2},idTransform,true,false},
		{timelineMenuState{count:2},idExport,true,false},
		{timelineMenuState{count:2},menuRevealClip,true,false},
		{timelineMenuState{count:1},menuRevealClip,true,true},
		{timelineMenuState{},idDelete,false,false},
		{timelineMenuState{},menuPasteHere,true,false},
		{timelineMenuState{pasteHere:true,gap:true},menuPasteHere,true,true},
		{timelineMenuState{gap:true},menuGapHere,true,true},
		{timelineMenuState{undo:true},idUndo,true,true},
		{timelineMenuState{hasClips:true},idSelectAll,true,true},
		{timelineMenuState{count:1,paste:true},idPaste,true,true},
	} {
		v,ok:=findMenuItem(timelineMenuItems(tc.state),tc.id)
		if ok!=tc.exists || v.enabled!=tc.enabled {t.Fatalf("%+v: %+v %t",tc,v,ok)}
	}
	for _,on:=range []bool{false,true} {
		items:=timelineMenuItems(timelineMenuState{snap:on,ripple:on})
		for _,id:=range []int{idSnap,idRippleTrim} {
			v,ok:=findMenuItem(items,id)
			if !ok || !v.enabled || v.checked!=on {t.Fatalf("toggle %d: %+v",id,v)}
		}
	}
}

func TestMediaContextEligibility(t *testing.T) {
	for _,selected:=range []bool{false,true} {
		items:=assetMenuItems(selected)
		for _,id:=range []int{idAdd,idRemoveAsset,menuRevealAsset} {
			v,ok:=findMenuItem(items,id)
			if !ok || v.enabled!=selected {t.Fatalf("asset selection %t action %d",selected,id)}
		}
		v,ok:=findMenuItem(items,idImport)
		if !ok || !v.enabled {t.Fatal("import unavailable")}
	}
}

func TestContextHit(t *testing.T) {
	for _,tc:=range []struct{x,y int32;audio,want bool}{
		{110,45,false,true},
		{110,110,true,true},
		{110,110,false,false},
		{110,20,true,false},
		{110,95,true,false},
		{160,45,true,false},
		{80,45,true,false},
	} {
		if got:=timelineMenuHit(tc.x,tc.y,100,160,tc.audio);got!=tc.want {t.Fatalf("%+v: %t",tc,got)}
	}
}
