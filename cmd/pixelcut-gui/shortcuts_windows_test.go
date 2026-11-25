package main

import "testing"

func TestShortcutMapping(t *testing.T) {
	for _,tc:=range []struct{
		key uintptr
		ctrl,shift,alt,editing,asset bool
		want int
	}{
		{0x20,false,false,false,false,false,idPlay},
		{0xbc,false,false,false,false,false,idFrameBack},
		{0xbe,false,false,false,false,false,idFrameForward},
		{0x25,true,false,false,false,false,idPrevEdit},
		{0x27,true,false,false,false,false,idNextEdit},
		{'I',false,false,false,false,false,idSetIn},
		{'O',false,false,false,false,false,idSetOut},
		{'B',false,false,false,false,false,idFrameSnap},
		{0xdb,false,false,false,false,false,idSpeedDown},
		{0xdd,false,false,false,false,false,idSpeedUp},
		{'I',false,false,false,true,false,keyNone},
		{0xbc,false,false,false,false,true,keyNone},
		{'C',true,false,false,false,false,idCopy},
		{'X',true,false,false,false,false,idCut},
		{'V',true,false,false,false,false,idPaste},
		{'D',true,false,false,false,false,idDuplicate},
		{'C',true,false,false,true,false,keyNone},
		{'X',true,false,false,true,false,keyNone},
		{'V',true,false,false,true,false,keyNone},
		{'D',true,false,false,true,false,keyNone},
		{'C',true,false,false,false,true,keyNone},
		{'X',true,false,false,false,true,keyNone},
		{'V',true,false,false,false,true,keyNone},
		{'D',true,false,false,false,true,keyNone},
		{'V',true,true,false,false,false,keyNone},
		{0x2e,false,true,false,false,false,idRippleDelete},
		{0x2e,false,true,false,false,true,keyNone},
		{0x2e,false,true,false,true,false,keyNone},
		{'G',false,false,false,false,false,idRemoveGap},
		{'R',false,false,false,false,false,idRippleTrim},
		{'R',false,false,false,false,true,keyNone},
		{'A',true,false,false,false,false,idSelectAll},
		{'A',true,false,false,false,true,keyNone},
		{'A',true,false,false,true,false,keyNone},
		{0x20,false,false,false,true,false,keyNone},
		{'Z',true,false,false,false,false,idUndo},
		{'Z',true,true,false,false,false,idRedo},
		{'Z',true,false,false,true,false,keyNone},
		{'S',true,false,false,false,false,idSave},
		{'S',false,false,false,false,false,idSplit},
		{'K',true,false,false,false,false,idSplit},
		{0x2e,false,false,false,false,true,idRemoveAsset},
		{'N',false,false,false,false,false,idSnap},
		{'N',false,false,false,false,true,keyNone},
		{0x2e,false,false,false,true,false,keyNone},
		{0x2e,false,false,false,false,false,idDelete},
		{0x0d,false,false,false,true,false,idApply},
		{0x09,false,true,false,true,false,keyPrevFocus},
		{0x25,false,false,false,false,false,keyMarkerBack},
		{0x25,false,false,false,true,false,keyNone},
		{0xbb,false,true,false,false,false,idZoomIn},
		{'F',false,false,true,false,false,keyNone},
		{0x70,false,false,false,true,false,idKeys},
	} {
		if got:=shortcutFor(tc.key,tc.ctrl,tc.shift,tc.alt,tc.editing,tc.asset);got!=tc.want {t.Fatalf("key %x: got %d, want %d",tc.key,got,tc.want)}
	}
}

func TestTimelineCursorHit(t *testing.T) {
	for _,tc:=range []struct{x,y,l,r int32;want string}{
		{100,50,100,180,"left"},
		{106,50,100,180,"left"},
		{107,50,100,180,"move"},
		{150,50,100,180,"move"},
		{173,50,100,180,"right"},
		{179,50,100,180,"right"},
		{180,50,100,180,""},
		{99,50,100,180,""},
		{100,31,100,180,""},
		{100,120,100,180,""},
		{100,50,100,115,"move"},
		{91,50,50,180,""},
		{92,50,50,180,"move"},
	}{
		if got:=videoHitMode(tc.x,tc.y,tc.l,tc.r);got!=tc.want {t.Fatalf("hit at %d,%d: %q want %q",tc.x,tc.y,got,tc.want)}
	}
	if cursorForMode("left",false)!=cursorSizeWE || cursorForMode("right",false)!=cursorSizeWE || cursorForMode("move",false)!=cursorSizeAll || cursorForMode("move",true)!=cursorNo {
		t.Fatal("unexpected cursor mapping")
	}
}
