package main

import (
	_ "embed"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"
)

//go:embed icon.png
var appIconPNG []byte

// ViewerApp controls the GUI state and rendering pipeline.
type ViewerApp struct {
	win           *mygo.Window
	tray          *mygo.Tray
	maskPath      string
	instMap       *InstanceMap
	baseImage     image.Image
	rendered      *ui.Bitmap
	mode          RenderMode
	modeStr       string
	opacity       float64
	zoomScale     float64
	colorSeed     uint32
	strokeWidth   int
	maskVisible   bool
	selectedID    uint32
	cellMetrics   map[uint32]*CellMetrics
	sortedCellIDs []uint32
	statusText    string
	hoverText     string
}

func (app *ViewerApp) buildTrayMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{
			Label: "显示主窗口",
			Click: func(*mygo.MenuItem, *mygo.Window) {
				app.win.Show()
				app.win.Focus()
			},
		},
		{
			Label: "隐藏主窗口",
			Click: func(*mygo.MenuItem, *mygo.Window) {
				app.win.Hide()
			},
		},
		mygo.Separator(),
		{
			Label: "打开文件...",
			Click: func(*mygo.MenuItem, *mygo.Window) {
				app.win.Show()
				app.win.Focus()
				app.openFileDialog()
			},
		},
		{
			Label: "打乱调色板",
			Click: func(*mygo.MenuItem, *mygo.Window) {
				app.colorSeed++
				app.refreshRender()
			},
		},
		{
			Label: "切换掩码显隐",
			Click: func(*mygo.MenuItem, *mygo.Window) {
				app.maskVisible = !app.maskVisible
				app.refreshRender()
			},
		},
		{
			Label: "显示模式",
			Submenu: []*mygo.MenuItem{
				{
					Label: "只显示轮廓",
					Click: func(*mygo.MenuItem, *mygo.Window) {
						app.mode = ModeOutline
						app.modeStr = "只显示轮廓"
						app.refreshRender()
					},
				},
				{
					Label: "半透明填充",
					Click: func(*mygo.MenuItem, *mygo.Window) {
						app.mode = ModeFill
						app.modeStr = "半透明填充"
						app.refreshRender()
					},
				},
				{
					Label: "填充加轮廓",
					Click: func(*mygo.MenuItem, *mygo.Window) {
						app.mode = ModeBoth
						app.modeStr = "填充加轮廓"
						app.refreshRender()
					},
				},
			},
		},
		mygo.Separator(),
		{
			Label: "检查更新...",
			Click: func(*mygo.MenuItem, *mygo.Window) {
				updater.CheckForUpdates()
			},
		},
		mygo.Separator(),
		{
			Label: "退出 CellMaskViewer",
			Click: func(*mygo.MenuItem, *mygo.Window) {
				mygo.App.Quit()
			},
		},
	})
}

func (app *ViewerApp) setupTray() {
	t, err := mygo.NewTray(mygo.TrayOptions{
		ID:      "cellmaskviewer-tray",
		Icon:    appIconPNG,
		ToolTip: "CellMaskViewer · 实例掩码查看器",
		Menu:    app.buildTrayMenu(),
	})
	if err != nil {
		panic(err)
	}
	app.tray = t
}

func (app *ViewerApp) selectCell(id uint32) {
	if app.selectedID == id {
		return
	}
	app.selectedID = id
	app.refreshRender()
}

func (app *ViewerApp) selectNextCell() {
	if len(app.sortedCellIDs) == 0 {
		return
	}
	if app.selectedID == 0 {
		app.selectCell(app.sortedCellIDs[0])
		return
	}
	idx, found := slices.BinarySearch(app.sortedCellIDs, app.selectedID)
	if !found {
		app.selectCell(app.sortedCellIDs[0])
		return
	}
	nextIdx := (idx + 1) % len(app.sortedCellIDs)
	app.selectCell(app.sortedCellIDs[nextIdx])
}

func (app *ViewerApp) selectPrevCell() {
	if len(app.sortedCellIDs) == 0 {
		return
	}
	if app.selectedID == 0 {
		app.selectCell(app.sortedCellIDs[len(app.sortedCellIDs)-1])
		return
	}
	idx, found := slices.BinarySearch(app.sortedCellIDs, app.selectedID)
	if !found {
		app.selectCell(app.sortedCellIDs[0])
		return
	}
	prevIdx := (idx - 1 + len(app.sortedCellIDs)) % len(app.sortedCellIDs)
	app.selectCell(app.sortedCellIDs[prevIdx])
}

func (app *ViewerApp) refreshRender() {
	if app.instMap == nil {
		return
	}
	opts := RenderOptions{
		Mode:        app.mode,
		Opacity:     app.opacity,
		Seed:        app.colorSeed,
		StrokeWidth: app.strokeWidth,
		MaskVisible: app.maskVisible,
		SelectedID:  app.selectedID,
	}
	rgba := RenderInstances(app.instMap, opts, app.baseImage)

	if app.zoomScale < 0.99 || app.zoomScale > 1.01 {
		sw := int(float64(rgba.Bounds().Dx()) * app.zoomScale)
		sh := int(float64(rgba.Bounds().Dy()) * app.zoomScale)
		scaled := image.NewRGBA(image.Rect(0, 0, sw, sh))
		for y := 0; y < sh; y++ {
			sy := int(float64(y) / app.zoomScale)
			for x := 0; x < sw; x++ {
				sx := int(float64(x) / app.zoomScale)
				scaled.Set(x, y, rgba.At(sx, sy))
			}
		}
		app.rendered = ui.NewBitmap(scaled)
	} else {
		app.rendered = ui.NewBitmap(rgba)
	}

	if app.win != nil {
		app.win.Invalidate()
	}
}

func (app *ViewerApp) loadMask(path string) {
	app.maskPath = path
	app.instMap = ReadTIFFMask(app.maskPath)
	app.cellMetrics = ComputeCellMetrics(app.instMap)
	app.sortedCellIDs = make([]uint32, 0, len(app.cellMetrics))
	for id := range app.cellMetrics {
		app.sortedCellIDs = append(app.sortedCellIDs, id)
	}
	slices.Sort(app.sortedCellIDs)
	app.selectedID = 0

	app.statusText = fmt.Sprintf("文件: %s | 尺寸: %dx%d | 实例总数: %d",
		filepath.Base(path), app.instMap.Width, app.instMap.Height, app.instMap.TotalCells)

	dir := filepath.Dir(app.maskPath)
	baseName := filepath.Base(app.maskPath)
	prefix := strings.Split(baseName, "_")[0]

	matches, _ := filepath.Glob(filepath.Join(dir, prefix+"*_source_*.jpg"))
	if len(matches) == 0 {
		matches, _ = filepath.Glob(filepath.Join(filepath.Dir(dir), prefix+"*_source_*.jpg"))
	}

	app.baseImage = nil
	if len(matches) > 0 {
		f, err := os.Open(matches[0])
		if err == nil {
			img, _, err := image.Decode(f)
			f.Close()
			if err == nil {
				app.baseImage = img
				app.statusText += fmt.Sprintf(" | 底图: %s", filepath.Base(matches[0]))
			}
		}
	}

	app.refreshRender()
}

func (app *ViewerApp) openFileDialog() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Title: "选择掩码 TIFF 文件或底图",
		Filters: []mygo.FileFilter{
			{Name: "Supported Images", Extensions: []string{"tif", "tiff", "jpg", "jpeg", "png"}},
		},
	})
	if err != nil {
		panic(err)
	}
	if len(paths) == 0 {
		return
	}
	app.handlePath(paths[0])
}

func (app *ViewerApp) handlePath(path string) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".tif" || ext == ".tiff" {
		app.loadMask(path)
	} else if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
		f, err := os.Open(path)
		if err != nil {
			panic(err)
		}
		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			panic(err)
		}
		app.baseImage = img
		app.refreshRender()
	}
}

func (app *ViewerApp) fitWindow() {
	if app.instMap == nil || app.win == nil {
		return
	}
	winW, winH := app.win.Size()
	availW := float64(winW - 32)
	availH := float64(winH - 130)
	if availW <= 0 || availH <= 0 {
		return
	}
	scaleX := availW / float64(app.instMap.Width)
	scaleY := availH / float64(app.instMap.Height)
	scale := scaleX
	if scaleY < scale {
		scale = scaleY
	}
	if scale > 1.0 {
		scale = 1.0
	}
	app.zoomScale = scale
	app.refreshRender()
}

func (app *ViewerApp) view(c *ui.Context) {
	ui.Column(c).Fill().Children(func() {
		// Control toolbar 1: Main operations with responsive wrapping
		ui.Row(c).Wrap().Padding(8, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, "打开文件").Clicked() {
				app.openFileDialog()
			}

			sel := ui.Select(c, &app.modeStr, []string{"填充加轮廓", "半透明填充", "只显示轮廓"}).Width(110)
			if sel.Changed() {
				switch app.modeStr {
				case "半透明填充":
					app.mode = ModeFill
				case "只显示轮廓":
					app.mode = ModeOutline
				default:
					app.mode = ModeBoth
				}
				app.refreshRender()
			}

			maskLabel := "掩码: 显示"
			if !app.maskVisible {
				maskLabel = "掩码: 隐藏"
			}
			if ui.Button(c, maskLabel).Clicked() {
				app.maskVisible = !app.maskVisible
				app.refreshRender()
			}

			if ui.Button(c, "打乱调色板").Clicked() {
				app.colorSeed++
				app.refreshRender()
			}

			strokeLabel := fmt.Sprintf("描边: %d像素", app.strokeWidth)
			if ui.Button(c, strokeLabel).Clicked() {
				if app.strokeWidth == 1 {
					app.strokeWidth = 2
				} else {
					app.strokeWidth = 1
				}
				app.refreshRender()
			}

			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "透明度:").FontSize(12).SingleLine()
				ui.Slider(c, &app.opacity, 0.1, 1.0).Width(80).OnChange(func() {
					app.refreshRender()
				})
			})

			if ui.Button(c, "100%").Clicked() {
				app.zoomScale = 1.0
				app.refreshRender()
			}
			if ui.Button(c, "50%").Clicked() {
				app.zoomScale = 0.5
				app.refreshRender()
			}
			if ui.Button(c, "25%").Clicked() {
				app.zoomScale = 0.25
				app.refreshRender()
			}
			if ui.Button(c, "适应窗口").Clicked() {
				app.fitWindow()
			}

			if ui.Button(c, "检查更新").Clicked() {
				updater.CheckForUpdates()
			}
		})

		// Control toolbar 2: Instance measurement & inspector card
		ui.Row(c).Wrap().Padding(6, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "实例测量:").FontSize(12).SingleLine()
			if ui.Button(c, "◀ 上一个").Clicked() {
				app.selectPrevCell()
			}
			if ui.Button(c, "下一个 ▶").Clicked() {
				app.selectNextCell()
			}
			if app.selectedID > 0 {
				if ui.Button(c, "取消选中").Clicked() {
					app.selectCell(0)
				}
				if cm, ok := app.cellMetrics[app.selectedID]; ok {
					bw := cm.MaxX - cm.MinX + 1
					bh := cm.MaxY - cm.MinY + 1
					info := fmt.Sprintf("【实例 #%d】 面积: %d 像素 | 外接尺寸: %d×%d 像素 | 范围: [%d, %d] - [%d, %d] | 形心: (%.1f, %.1f)",
						cm.ID, cm.Area, bw, bh, cm.MinX, cm.MinY, cm.MaxX, cm.MaxY, cm.CentroidX, cm.CentroidY)
					ui.Text(c, info).FontSize(12).SingleLine()
				}
			} else {
				ui.Text(c, "提示: 点击图像中任意细胞即可选中高亮并读取测量参数").FontSize(12).SingleLine()
			}

			if app.hoverText != "" {
				ui.Text(c, "| "+app.hoverText).FontSize(12).SingleLine()
			}
			if app.statusText != "" {
				ui.Text(c, "| "+app.statusText).FontSize(12).SingleLine()
			}
		})

		// Viewport area
		if app.rendered != nil {
			ui.ScrollBoth(c).Grow(1).Children(func() {
				imgElem := ui.Image(c, app.rendered).Cursor(ui.CursorCrosshair)
				if px, py, over := imgElem.PointerPosition(); over && app.instMap != nil {
					origX := int(float64(px) / app.zoomScale)
					origY := int(float64(py) / app.zoomScale)
					if origX >= 0 && origX < app.instMap.Width && origY >= 0 && origY < app.instMap.Height {
						hoverID := app.instMap.Labels[origY*app.instMap.Width+origX]
						if hoverID > 0 {
							app.hoverText = fmt.Sprintf("坐标: (%d, %d) | 悬停实例: #%d", origX, origY, hoverID)
						} else {
							app.hoverText = fmt.Sprintf("坐标: (%d, %d) | 背景", origX, origY)
						}
					}
					if imgElem.Clicked() {
						if origX >= 0 && origX < app.instMap.Width && origY >= 0 && origY < app.instMap.Height {
							clickedID := app.instMap.Labels[origY*app.instMap.Width+origX]
							app.selectCell(clickedID)
						}
					}
				}
			})
		} else {
			ui.Column(c).Grow(1).Center().Gap(14).Children(func() {
				ui.Text(c, "可以直接拖拽 AIsoftware2 的 .tif 掩码文件到此窗口直接查看").FontSize(18).Bold().TextAlign(ui.Center)
				ui.Text(c, "支持单页大图矩阵、多页局部包围盒裁剪掩码、自动关联配准 _source_BF 底图").FontSize(13).TextAlign(ui.Center)
			})
		}
	})
}

func main() {
	app := &ViewerApp{
		mode:        ModeBoth,
		modeStr:     "填充加轮廓",
		opacity:     0.5,
		zoomScale:   1.0,
		colorSeed:   0,
		strokeWidth: 1,
		maskVisible: true,
		selectedID:  0,
		statusText:  "就绪：支持直接拖拽 .tif 文件到窗口",
		hoverText:   "",
	}

	// Handle initial command line argument if supplied
	if len(os.Args) > 1 {
		app.maskPath = os.Args[1]
	}

	useMirroredUpdates()
	mygo.Use(updater.New(updater.Options{
		Icon:     appIconPNG,
		Language: "zh-Hans",
	}))

	mygo.App.SetName("CellMaskViewer")
	mygo.App.WhenReady(func() {
		app.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "AIsoftware2 TIFF 实例掩码查看器",
			Width:     1280,
			Height:    900,
			MinWidth:  800,
			MinHeight: 600,
			Content:   ui.View(app.view),
		})
		_ = app.win.SetIcon(appIconPNG)

		app.setupTray()

		app.win.OnClose(func(e *mygo.CloseEvent) {
			e.PreventDefault()
			app.win.Hide()
		})

		// Register drag and drop callback
		app.win.OnFileDrop(func(e *mygo.FileDropEvent) {
			if len(e.Paths) > 0 {
				app.handlePath(e.Paths[0])
			}
		})

		if app.maskPath != "" {
			app.loadMask(app.maskPath)
		}
	})

	mygo.App.OnWindowAllClosed(func() {})

	if err := mygo.App.Run(); err != nil {
		panic(err)
	}
}
