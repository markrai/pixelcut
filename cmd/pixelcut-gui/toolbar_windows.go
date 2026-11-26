package main

type toolbarItem struct {
	label        string
	x, width, id int
}

var mainToolbarItems = [...]toolbarItem{
	{"New", 12, 60, idNew},
	{"Open", 78, 60, idOpen},
	{"Save", 144, 60, idSave},
	{"Save As", 210, 70, idSaveAs},
	{"Recent", 286, 68, idRecent},
	{"Close", 360, 60, idCloseProject},
	{"Undo", 426, 58, idUndo},
	{"Redo", 490, 58, idRedo},
	{"Shortcuts", 554, 88, idKeys},
	{"Settings", 648, 90, idPreferences},
	{"Cache", 744, 65, idMediaCache},
	{"Relink", 815, 78, idRelinkMenu},
	{"Export...", 970, 126, idExportMenu},
}

func mainToolbarX(id, x, width, clientWidth int) int {
	if id == idExportMenu {
		return max(899, clientWidth-width-12)
	}
	return x
}
