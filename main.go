package main

import (
	_ "embed"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"
)

//go:embed icon.png
var appIconPNG []byte

// ViewerApp controls the GUI state and rendering pipeline.
type ViewerApp struct {
	win        *mygo.Window
	tray       *mygo.Tray
	maskPath   string
	instMap    *InstanceMap
	baseImage  image.Image
	rendered   *ui.Bitmap
	mode       RenderMode
	opacity    float64
	zoomScale  float64
	statusText string
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
			Label: "显示模式",
			Submenu: []*mygo.MenuItem{
				{
					Label: "只显示轮廓",
					Click: func(*mygo.MenuItem, *mygo.Window) {
						app.mode = ModeOutline
						app.refreshRender()
					},
				},
				{
					Label: "半透明填充",
					Click: func(*mygo.MenuItem, *mygo.Window) {
						app.mode = ModeFill
						app.refreshRender()
					},
				},
				{
					Label: "填充加轮廓",
					Click: func(*mygo.MenuItem, *mygo.Window) {
						app.mode = ModeBoth
						app.refreshRender()
					},
				},
			},
		},
		mygo.Separator(),
		updater.MenuItem(),
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

func (app *ViewerApp) refreshRender() {
	if app.instMap == nil {
		return
	}
	rgba := RenderInstances(app.instMap, app.mode, app.opacity, app.baseImage)

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

func (app *ViewerApp) view(c *ui.Context) {
	ui.Column(c).Fill().Children(func() {
		// Control toolbar
		ui.Row(c).Padding(10).Gap(8).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, "打开文件").Clicked() {
				app.openFileDialog()
			}

			if ui.Button(c, "半透明填充").Clicked() {
				app.mode = ModeFill
				app.refreshRender()
			}
			if ui.Button(c, "只显示轮廓").Clicked() {
				app.mode = ModeOutline
				app.refreshRender()
			}
			if ui.Button(c, "填充加轮廓").Clicked() {
				app.mode = ModeBoth
				app.refreshRender()
			}

			ui.Text(c, "透明度:").FontSize(12)
			ui.Slider(c, &app.opacity, 0.1, 1.0).OnChange(func() {
				app.refreshRender()
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

			if ui.Button(c, "检查更新").Clicked() {
				updater.CheckForUpdates()
			}

			ui.Text(c, app.statusText).Grow(1).FontSize(12).SingleLine()
		})

		// Viewport area
		ui.Scroll(c).Grow(1).Children(func() {
			if app.rendered != nil {
				ui.Image(c, app.rendered)
			} else {
				ui.Column(c).Fill().AlignItems(ui.Center).Padding(60).Children(func() {
					ui.Text(c, "可以直接拖拽 AIsoftware2 的 .tif 掩码文件到此窗口直接查看").FontSize(15)
					ui.Text(c, "支持单页大图矩阵、多页局部包围盒裁剪掩码、自动关联配准 _source_BF 底图").FontSize(12)
				})
			}
		})
	})
}

func main() {
	app := &ViewerApp{
		mode:       ModeBoth,
		opacity:    0.5,
		zoomScale:  1.0,
		statusText: "就绪：支持直接拖拽 .tif 文件到窗口",
	}

	// Handle initial command line argument if supplied
	if len(os.Args) > 1 {
		app.maskPath = os.Args[1]
	}

	useMirroredUpdates()
	mygo.Use(updater.New(updater.Options{Icon: appIconPNG}))

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
