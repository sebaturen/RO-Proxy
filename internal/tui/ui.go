package tui

import (
	"fmt"
	"os"
	"roproxy/internal/common"
	"strconv"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (d *Dashboard) buildUI() {
	d.buildPanels()
	d.buildLayout()
	d.setupKeyBindings()
	d.updateControlsView()
	d.updateStatusBar()
}

func (d *Dashboard) buildPanels() {
	d.statsView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(false)
	d.statsView.SetBorder(true).
		SetTitle(" Statistics ").
		SetTitleAlign(tview.AlignLeft).
		SetBorderPadding(0, 0, 1, 1)
	
	d.logsView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetMaxLines(1000).
		SetChangedFunc(func() { d.logsView.ScrollToEnd() })
	d.logsView.SetBorder(true).
		SetTitle(" Logs ").
		SetTitleAlign(tview.AlignLeft).
		SetBorderPadding(0, 0, 1, 1)
	
	d.connectionsView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true)
	d.connectionsView.SetBorder(true).
		SetTitle(" Active Connections ").
		SetTitleAlign(tview.AlignLeft).
		SetBorderPadding(0, 0, 1, 1)
	
	d.controlsText = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(false)
	
	d.buildFilterInput()
	
	d.controlsView = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(d.controlsText, 0, 1, false).
		AddItem(d.filterInput, 1, 0, false)
	d.controlsView.SetBorder(true).
		SetTitle(" Controls ").
		SetTitleAlign(tview.AlignLeft).
		SetBorderPadding(0, 0, 1, 1)
	
	d.statusBar = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)
}

func (d *Dashboard) buildFilterInput() {
	d.filterInput = tview.NewInputField().
		SetLabel("").
		SetFieldWidth(8).
		SetAcceptanceFunc(tview.InputFieldInteger).
		SetFieldBackgroundColor(tcell.ColorBlack).
		SetFieldTextColor(tcell.ColorYellow).
		SetDoneFunc(func(key tcell.Key) {
			go func() {
				switch key {
				case tcell.KeyEnter:
					text := d.filterInput.GetText()
					if text == "" {
						d.connectionFilter = 0
					} else if connID, err := strconv.ParseUint(text, 10, 64); err == nil {
						d.connectionFilter = connID
					}
					d.filterActive = false
					d.app.QueueUpdateDraw(func() {
						d.updateControlsView()
						d.controlsView.SetBorder(true)
						d.app.SetFocus(d.logsView)
					})
					common.Log(common.LogUI, common.LogInfo, "[yellow]Connection filter: %s[-]", colorBool(d.connectionFilter > 0, fmt.Sprintf("#%d", d.connectionFilter), "ALL"))
				case tcell.KeyEscape:
					d.filterActive = false
					d.app.QueueUpdateDraw(func() {
						d.updateControlsView()
						d.controlsView.SetBorder(true)
						d.app.SetFocus(d.logsView)
					})
				}
			}()
		})
}

func (d *Dashboard) buildLayout() {
	leftPanel := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(d.statsView, 11, 0, false).
		AddItem(d.connectionsView, 0, 1, false).
		AddItem(d.controlsView, 14, 0, false)

	d.rootFlex = tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(leftPanel, 40, 0, false).
		AddItem(d.logsView, 0, 1, false)

	mainLayout := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(d.rootFlex, 0, 1, true).
		AddItem(d.statusBar, 1, 0, false)

	d.app.SetRoot(mainLayout, true)
}

func (d *Dashboard) setupKeyBindings() {
	d.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlC || event.Key() == tcell.KeyCtrlD {
			go func() {
				d.app.Stop()
				time.Sleep(100 * time.Millisecond)
				os.Exit(0)
			}()
			return nil
		}
		
		if d.filterActive {
			return event
		}
		
		switch event.Rune() {
		case 'v', 'V':
			go d.toggleVerbosity()
		case 'f', 'F':
			go d.promptConnectionFilter()
		case 'l', 'L':
			go d.clearLogs()
		case 'r', 'R':
			go d.toggleRecording()
		case 'q', 'Q':
			go func() {
				d.app.Stop()
				time.Sleep(100 * time.Millisecond)
				os.Exit(0)
			}()
		default:
			return event
		}
		return nil
	})
}
