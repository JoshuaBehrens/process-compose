package tui

import (
	"fmt"
	"github.com/f1bonacc1/process-compose/src/config"
	"github.com/f1bonacc1/process-compose/src/updater"
	"path/filepath"
	"time"

	"github.com/f1bonacc1/process-compose/src/app"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type FullScrState int

const (
	LogFull     FullScrState = 0
	ProcFull                 = 1
	LogProcHalf              = 2
)

const (
	PageMain   = "main"
	PageDialog = "dialog"
)

var pcv *pcView = nil
var scFiles = []string{
	"shortcuts.yaml",
	"shortcuts.yml",
}

type pcView struct {
	procTable     *tview.Table
	statTable     *tview.Table
	appView       *tview.Application
	logsText      *LogView
	statusText    *tview.TextView
	helpText      *tview.TextView
	pages         *tview.Pages
	procNames     []string
	logFollow     bool
	logSelect     bool
	fullScrState  FullScrState
	loggedProc    string
	shortcuts     ShortCuts
	procCountCell *tview.TableCell
	mainGrid      *tview.Grid
	logsTextArea  *tview.TextArea
}

func newPcView(logLength int) *pcView {
	//_ = pv.shortcuts.loadFromFile("short-cuts-new.yaml")
	pv := &pcView{
		appView:       tview.NewApplication(),
		logsText:      NewLogView(logLength),
		statusText:    tview.NewTextView().SetDynamicColors(true),
		procNames:     app.PROJ.GetLexicographicProcessNames(),
		logFollow:     true,
		fullScrState:  LogProcHalf,
		helpText:      tview.NewTextView().SetDynamicColors(true),
		loggedProc:    "",
		shortcuts:     getDefaultActions(),
		procCountCell: tview.NewTableCell(""),
		mainGrid:      tview.NewGrid(),
		logsTextArea:  tview.NewTextArea(),
		logSelect:     false,
	}
	pv.loadShortcuts()
	pv.procTable = pv.createProcTable()
	pv.statTable = pv.createStatTable()
	pv.updateHelpTextView()
	pv.createGrid()
	pv.createLogSelectionTextArea()
	pv.pages = tview.NewPages().
		AddPage(PageMain, pv.mainGrid, true, true)

	pv.appView.SetRoot(pv.pages, true).EnableMouse(true).SetInputCapture(pv.onAppKey)
	if len(pv.procNames) > 0 {
		name := pv.procNames[0]
		pv.logsText.SetTitle(name)
		pv.followLog(name)
	}
	return pv
}

func (pv *pcView) loadShortcuts() {
	for _, path := range scFiles {
		if err := pv.shortcuts.loadFromFile(filepath.Join(config.ProcCompHome(), path)); err == nil {
			break
		}
	}
}

func (pv *pcView) onAppKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case pv.shortcuts.ShortCutKeys[ActionQuit].key:
		pv.terminateAppView()
	case pv.shortcuts.ShortCutKeys[ActionLogScreen].key:
		if pv.fullScrState == LogFull {
			pv.fullScrState = LogProcHalf
		} else {
			pv.fullScrState = LogFull
		}
		pv.redrawGrid()
		pv.updateHelpTextView()
	case pv.shortcuts.ShortCutKeys[ActionFollowLog].key:
		pv.toggleLogFollow()
	case pv.shortcuts.ShortCutKeys[ActionWrapLog].key:
		pv.logsText.ToggleWrap()
		pv.updateHelpTextView()
	case pv.shortcuts.ShortCutKeys[ActionLogSelection].key:
		pv.stopFollowLog()
		pv.toggleLogSelection()
		pv.appView.SetFocus(pv.logsTextArea)
		pv.updateHelpTextView()
	case pv.shortcuts.ShortCutKeys[ActionProcessScreen].key:
		if pv.fullScrState == ProcFull {
			pv.fullScrState = LogProcHalf
		} else {
			pv.fullScrState = ProcFull
		}
		pv.redrawGrid()
		pv.onProcRowSpanChange()
		pv.updateHelpTextView()
	case tcell.KeyCtrlC:
		pv.terminateAppView()
	case pv.shortcuts.ShortCutKeys[ActionProcessInfo].key:
		pv.showInfo()
	default:
		return event
	}
	return nil
}

func (pv *pcView) terminateAppView() {

	m := tview.NewModal().
		SetText("Are you sure you want to quit?\nThis will terminate all the running processes.").
		AddButtons([]string{"Quit", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Quit" {
				go pv.handleShutDown()
			}
			pv.pages.SwitchToPage(PageMain)
			pv.pages.RemovePage(PageDialog)
		})
	// Display and focus the dialog
	pv.pages.AddPage(PageDialog, createDialogPage(m, 50, 50), true, true)
}

func (pv *pcView) showError(errMessage string) {
	m := tview.NewModal().
		SetText(fmt.Sprintf("Error: [white:red]%s[-:-:-]", errMessage)).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			pv.pages.SwitchToPage(PageMain)
			pv.pages.RemovePage(PageDialog)
		})

	pv.pages.AddPage(PageDialog, createDialogPage(m, 50, 50), true, true)
}

func (pv *pcView) showInfo() {
	name := pv.getSelectedProcName()
	info, err := app.PROJ.GetProcessInfo(name)
	if err != nil {
		pv.showError(err.Error())
		return
	}
	form := pv.createProcInfoForm(info)
	pv.showDialog(form)
}

func (pv *pcView) handleShutDown() {
	pv.statTable.SetCell(0, 2, tview.NewTableCell("Shutting Down...").
		SetSelectable(false).
		SetAlign(tview.AlignCenter).
		SetExpansion(0).
		SetTextColor(tcell.ColorWhite).
		SetBackgroundColor(tcell.ColorRed))
	app.PROJ.ShutDownProject()
	time.Sleep(time.Second)
	pv.appView.Stop()

}

func (pv *pcView) getSelectedProcName() string {
	if pv.procTable == nil {
		return ""
	}
	row, _ := pv.procTable.GetSelection()
	if row > 0 && row <= len(pv.procNames) {
		return pv.procNames[row-1]
	}
	return ""
}

func (pv *pcView) onProcRowSpanChange() {
	if pv.fullScrState == ProcFull && pv.logFollow {
		pv.stopFollowLog()
	}
}

func (pv *pcView) updateHelpTextView() {
	logScrBool := pv.fullScrState != LogFull
	procScrBool := pv.fullScrState != ProcFull
	pv.helpText.Clear()
	fmt.Fprintf(pv.helpText, "%s ", "[lightskyblue:]LOGS:[-:-:-]")
	pv.shortcuts.ShortCutKeys[ActionLogScreen].writeToggleButton(pv.helpText, logScrBool)
	pv.shortcuts.ShortCutKeys[ActionFollowLog].writeToggleButton(pv.helpText, !pv.logFollow)
	pv.shortcuts.ShortCutKeys[ActionWrapLog].writeToggleButton(pv.helpText, !pv.logsText.IsWrapOn())
	pv.shortcuts.ShortCutKeys[ActionLogSelection].writeToggleButton(pv.helpText, !pv.logSelect)
	fmt.Fprintf(pv.helpText, "%s ", "[lightskyblue::b]PROCESS:[-:-:-]")
	pv.shortcuts.ShortCutKeys[ActionProcessInfo].writeButton(pv.helpText)
	pv.shortcuts.ShortCutKeys[ActionProcessStart].writeButton(pv.helpText)
	pv.shortcuts.ShortCutKeys[ActionProcessScreen].writeToggleButton(pv.helpText, procScrBool)
	pv.shortcuts.ShortCutKeys[ActionProcessStop].writeButton(pv.helpText)
	pv.shortcuts.ShortCutKeys[ActionProcessRestart].writeButton(pv.helpText)
	pv.shortcuts.ShortCutKeys[ActionQuit].writeButton(pv.helpText)
}

func (pv *pcView) runOnce() {
	version, err := updater.GetLatestReleaseName()
	if err != nil {
		return
	}
	if config.Version != version {
		pv.showUpdateAvailable(version)
	}
}

func (pv *pcView) showUpdateAvailable(version string) {
	pv.appView.QueueUpdateDraw(func() {
		pv.statTable.GetCell(0, 1).SetText(fmt.Sprintf("%s  [green:]%s[-:-:-]", config.Version, version))
	})
}

func SetupTui(logLength int) {

	pv := newPcView(logLength)

	go pv.updateTable()
	go pv.updateLogs()
	if config.CheckForUpdates == "true" {
		go pv.runOnce()
	}

	pcv = pv
	if err := pv.appView.Run(); err != nil {
		panic(err)
	}
}

func Stop() {
	if pcv != nil {
		pcv.handleShutDown()
	}
}
