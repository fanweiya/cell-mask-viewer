package main

import (
	_ "embed"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"math"
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

const appVersion = "0.1.3"

// ViewerApp controls the GUI state and rendering pipeline.
type ViewerApp struct {
	win               *mygo.Window
	tray              *mygo.Tray
	maskPath          string
	instMap           *InstanceMap
	baseImage         image.Image
	renderedRGBA      *image.RGBA
	rendered          *ui.Bitmap
	placeholderBitmap *ui.Bitmap
	canvasScroll      ui.ScrollState
	mode              RenderMode
	modeStr           string
	opacity           float64
	zoomScale         float64
	colorSeed         uint32
	strokeWidth       int
	maskVisible       bool
	selectedID        uint32
	cellMetrics       map[uint32]*CellMetrics
	sortedCellIDs     []uint32
	statusText        string
	hoverText         string
	quitting          bool
}

func (app *ViewerApp) buildTrayMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{
			Label:    "CellMaskViewer v" + appVersion,
			Disabled: true,
		},
		mygo.Separator(),
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
				app.quitting = true
				if app.tray != nil {
					app.tray.Destroy()
				}
				mygo.App.Exit(0)
			},
		},
	})
}

func (app *ViewerApp) setupTray() {
	t, err := mygo.NewTray(mygo.TrayOptions{
		ID:      "cellmaskviewer-tray",
		Icon:    appIconPNG,
		ToolTip: "CellMaskViewer v" + appVersion + " · 实例掩码查看器",
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
	app.renderedRGBA = rgba

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
	instMap, err := ReadTIFFMask(path)
	if err != nil {
		app.statusText = fmt.Sprintf("打开失败: %s (%v)", filepath.Base(path), err)
		if app.win != nil {
			app.win.Invalidate()
		}
		return
	}
	app.maskPath = path
	app.instMap = instMap
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
	if app.win != nil {
		app.fitWindow()
	}
}

func (app *ViewerApp) zoomIn() {
	app.zoomScale *= 1.25
	if app.zoomScale > 8.0 {
		app.zoomScale = 8.0
	}
	app.refreshRender()
}

func (app *ViewerApp) zoomOut() {
	app.zoomScale /= 1.25
	if app.zoomScale < 0.05 {
		app.zoomScale = 0.05
	}
	app.refreshRender()
}

func generateCellPlaceholderBitmap() *ui.Bitmap {
	w, h := 280, 180
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))

	bg := color.RGBA{R: 246, G: 249, B: 253, A: 255}
	cardBorder := color.RGBA{R: 215, G: 228, B: 242, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x == 0 || x == w-1 || y == 0 || y == h-1 {
				rgba.Set(x, y, cardBorder)
			} else {
				rgba.Set(x, y, bg)
			}
		}
	}

	// Draw subtle microscope circular field grid (dashed circle)
	cx, cy := w/2, h/2-6
	r := 68
	dishBorder := color.RGBA{R: 200, G: 215, B: 235, A: 255}
	for deg := 0; deg < 360; deg += 3 {
		if (deg/12)%2 == 0 {
			rad := float64(deg) * math.Pi / 180.0
			px := cx + int(float64(r)*math.Cos(rad))
			py := cy + int(float64(r)*math.Sin(rad))
			if px >= 0 && px < w && py >= 0 && py < h {
				rgba.Set(px, py, dishBorder)
			}
		}
	}

	// Helper to draw an ellipse cell with nucleus
	drawCell := func(ox, oy, rx, ry int, angle float64, fill, border, nucleus color.RGBA) {
		cosA := math.Cos(angle)
		sinA := math.Sin(angle)
		for y := -ry - 2; y <= ry+2; y++ {
			for x := -rx - 2; x <= rx+2; x++ {
				xr := float64(x)*cosA + float64(y)*sinA
				yr := -float64(x)*sinA + float64(y)*cosA
				d := (xr*xr)/float64(rx*rx) + (yr*yr)/float64(ry*ry)
				px := ox + x
				py := oy + y
				if px < 0 || px >= w || py < 0 || py >= h {
					continue
				}
				if d <= 1.0 {
					if d >= 0.75 {
						rgba.Set(px, py, border)
					} else {
						rgba.Set(px, py, fill)
					}
				}
			}
		}
		// Nucleus in center
		nr := 4
		for ny := -nr; ny <= nr; ny++ {
			for nx := -nr; nx <= nr; nx++ {
				if nx*nx+ny*ny <= nr*nr {
					px := ox + nx
					py := oy + ny
					if px >= 0 && px < w && py >= 0 && py < h {
						rgba.Set(px, py, nucleus)
					}
				}
			}
		}
	}

	// Draw 5 stylized cells with instance segmentation colors
	// 1. Turquoise / Teal Cell
	drawCell(cx-34, cy-22, 22, 16, 0.3,
		color.RGBA{R: 190, G: 242, B: 235, A: 255},
		color.RGBA{R: 20, G: 165, B: 150, A: 255},
		color.RGBA{R: 10, G: 110, B: 100, A: 255})

	// 2. Violet / Purple Cell
	drawCell(cx+28, cy-20, 20, 18, -0.4,
		color.RGBA{R: 230, G: 220, B: 255, A: 255},
		color.RGBA{R: 120, G: 80, B: 230, A: 255},
		color.RGBA{R: 70, G: 35, B: 170, A: 255})

	// 3. Amber / Golden Cell (Selected instance with bounding box)
	selX, selY, selRx, selRy := cx-8, cy+22, 24, 18
	drawCell(selX, selY, selRx, selRy, 0.1,
		color.RGBA{R: 255, G: 238, B: 200, A: 255},
		color.RGBA{R: 240, G: 150, B: 20, A: 255},
		color.RGBA{R: 180, G: 90, B: 10, A: 255})

	// 4. Emerald Green Cell
	drawCell(cx+36, cy+26, 18, 14, 0.5,
		color.RGBA{R: 215, G: 248, B: 220, A: 255},
		color.RGBA{R: 35, G: 175, B: 85, A: 255},
		color.RGBA{R: 20, G: 115, B: 55, A: 255})

	// 5. Sky Blue Cell
	drawCell(cx-46, cy+18, 17, 13, -0.2,
		color.RGBA{R: 215, G: 235, B: 255, A: 255},
		color.RGBA{R: 45, G: 135, B: 245, A: 255},
		color.RGBA{R: 25, G: 85, B: 180, A: 255})

	// Bounding Box around the Amber selected cell (Instance detection bounding box)
	bx0, by0 := selX-selRx-4, selY-selRy-4
	bx1, by1 := selX+selRx+4, selY+selRy+4
	boxCol := color.RGBA{R: 230, G: 140, B: 10, A: 255}
	for x := bx0; x <= bx1; x++ {
		if (x/3)%2 == 0 {
			rgba.Set(x, by0, boxCol)
			rgba.Set(x, by1, boxCol)
		}
	}
	for y := by0; y <= by1; y++ {
		if (y/3)%2 == 0 {
			rgba.Set(bx0, y, boxCol)
			rgba.Set(bx1, y, boxCol)
		}
	}
	// Corner handles on bounding box
	for _, pt := range [][2]int{{bx0, by0}, {bx1, by0}, {bx0, by1}, {bx1, by1}} {
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				rgba.Set(pt[0]+dx, pt[1]+dy, boxCol)
			}
		}
	}

	return ui.NewBitmap(rgba)
}

func (app *ViewerApp) openFileDialog() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Title: "选择掩码 TIFF 文件或底图",
		Filters: []mygo.FileFilter{
			{Name: "Supported Images", Extensions: []string{"tif", "tiff", "jpg", "jpeg", "png"}},
		},
	})
	if err != nil {
		app.statusText = fmt.Sprintf("打开文件对话框失败: %v", err)
		if app.win != nil {
			app.win.Invalidate()
		}
		return
	}
	if len(paths) == 0 {
		return
	}
	app.handlePath(paths[0])
}

func (app *ViewerApp) exportPNG() {
	if app.renderedRGBA == nil {
		app.statusText = "当前无渲染结果可导出"
		if app.win != nil {
			app.win.Invalidate()
		}
		return
	}
	defaultName := "mask_rendered.png"
	defaultDir := ""
	if app.maskPath != "" {
		base := strings.TrimSuffix(filepath.Base(app.maskPath), filepath.Ext(app.maskPath))
		defaultName = base + "_rendered.png"
		defaultDir = filepath.Dir(app.maskPath)
	}
	defaultPath := filepath.Join(defaultDir, defaultName)

	savePath, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Title:       "导出渲染图为 PNG",
		DefaultPath: defaultPath,
		Filters: []mygo.FileFilter{
			{Name: "PNG Image", Extensions: []string{"png"}},
		},
	})
	if err != nil || savePath == "" {
		return
	}
	if !strings.HasSuffix(strings.ToLower(savePath), ".png") {
		savePath += ".png"
	}

	outFile, err := os.Create(savePath)
	if err != nil {
		app.statusText = fmt.Sprintf("创建导出文件失败: %v", err)
		if app.win != nil {
			app.win.Invalidate()
		}
		return
	}
	defer outFile.Close()

	if err := png.Encode(outFile, app.renderedRGBA); err != nil {
		app.statusText = fmt.Sprintf("保存 PNG 失败: %v", err)
	} else {
		app.statusText = fmt.Sprintf("已成功导出图像: %s", filepath.Base(savePath))
	}
	if app.win != nil {
		app.win.Invalidate()
	}
}

func (app *ViewerApp) handlePath(path string) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".tif" || ext == ".tiff" {
		app.loadMask(path)
	} else if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
		f, err := os.Open(path)
		if err != nil {
			app.statusText = fmt.Sprintf("打开底图失败: %v", err)
			if app.win != nil {
				app.win.Invalidate()
			}
			return
		}
		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			app.statusText = fmt.Sprintf("解码底图失败: %v", err)
			if app.win != nil {
				app.win.Invalidate()
			}
			return
		}
		app.baseImage = img
		app.refreshRender()
	} else {
		app.statusText = fmt.Sprintf("不支持的文件格式: %s", filepath.Base(path))
		if app.win != nil {
			app.win.Invalidate()
		}
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
			if ui.Button(c, "导出 PNG...").Clicked() {
				app.exportPNG()
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
			ui.ScrollBoth(c).TrackScroll(&app.canvasScroll).Grow(1).HandleInput(func(ev ui.InputEvent) bool {
				if ev.Kind == ui.InputScroll {
					if ev.DY < 0 {
						app.zoomIn()
						return true
					} else if ev.DY > 0 {
						app.zoomOut()
						return true
					}
				}
				if ev.Kind == ui.InputZoom {
					if ev.Scale > 1.0 {
						app.zoomIn()
						return true
					} else if ev.Scale < 1.0 {
						app.zoomOut()
						return true
					}
				}
				if ev.Kind == ui.InputKeyDown {
					switch ev.Key {
					case ui.KeyEqual:
						app.zoomIn()
						return true
					case ui.KeyMinus:
						app.zoomOut()
						return true
					case ui.Key0:
						app.fitWindow()
						return true
					case ui.Key1:
						app.zoomScale = 1.0
						app.refreshRender()
						return true
					case ui.KeyW, ui.KeyUp:
						app.canvasScroll.Y -= 64
						if app.canvasScroll.Y < 0 {
							app.canvasScroll.Y = 0
						}
						return true
					case ui.KeyS, ui.KeyDown:
						app.canvasScroll.Y += 64
						return true
					case ui.KeyA, ui.KeyLeft:
						app.canvasScroll.X -= 64
						if app.canvasScroll.X < 0 {
							app.canvasScroll.X = 0
						}
						return true
					case ui.KeyD, ui.KeyRight:
						app.canvasScroll.X += 64
						return true
					}
				}
				return false
			}).Children(func() {
				imgElem := ui.Image(c, app.rendered).Cursor(ui.CursorCrosshair).HandleInput(func(ev ui.InputEvent) bool {
					if ev.Kind == ui.InputScroll {
						if ev.DY < 0 {
							app.zoomIn()
							return true
						} else if ev.DY > 0 {
							app.zoomOut()
							return true
						}
					}
					return false
				})
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
				if app.placeholderBitmap != nil {
					ui.Image(c, app.placeholderBitmap)
				}
				ui.Text(c, "TIFF 图像与细胞实例掩码查看器").FontSize(18).Bold().TextAlign(ui.Center)
				ui.Text(c, "专为生物显微扫描、多实例细胞分割 (Multimasks) 与明场底图配准复核设计").FontSize(12).TextColor(ui.RGB(130, 145, 165)).TextAlign(ui.Center)

				if ui.Button(c, "📂 打开 TIFF 掩码文件...").Clicked() {
					app.openFileDialog()
				}

				ui.Text(c, "支持格式: 单页大图矩阵 · 多页局部包围盒裁剪掩码 · 自动关联同名 _source_BF 底图").FontSize(11).TextColor(ui.RGB(150, 165, 185)).TextAlign(ui.Center)
				ui.Text(c, "交互提示: 直接拖拽 .tif 文件到窗口 | 鼠标滚轮连续缩放 | 键盘 W/A/S/D 移动视口 | 点击细胞读取形心与面积").FontSize(11).TextColor(ui.RGB(160, 175, 195)).TextAlign(ui.Center)
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
	app.placeholderBitmap = generateCellPlaceholderBitmap()

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
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		app.quitting = true
	})
	mygo.App.WhenReady(func() {
		app.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "AIsoftware2 TIFF 实例掩码查看器 v" + appVersion,
			Width:     1280,
			Height:    900,
			MinWidth:  800,
			MinHeight: 600,
			Content:   ui.View(app.view),
		})
		_ = app.win.SetIcon(appIconPNG)

		app.setupTray()

		app.win.OnClose(func(e *mygo.CloseEvent) {
			if !app.quitting {
				e.PreventDefault()
				app.win.Hide()
			}
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
