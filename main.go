package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
)

type Config struct {
	Port       string `json:"port"`
	ScriptPath string `json:"script_path"`
}

var (
	defaultConfig  = Config{Port: "8080", ScriptPath: "/home/dylan/Documents/hibernate.sh"}
	config         Config
	configPath     string
	iconPath       string
	serverInstance *http.Server
	serverMutex    sync.Mutex
	statusLabel    *widget.Label
	lastActionTime time.Time
	myApp          fyne.App
	myWindow       fyne.Window
	iconRes        fyne.Resource
	compactSize    = fyne.NewSize(380, 160)
	expandedSize   = fyne.NewSize(550, 450)
)

func generateIcon(path string) error {
	const size = 256
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	col := color.NRGBA{80, 80, 80, 255}

	drawThickPoint := func(x, y int, thick int) {
		for i := -thick; i <= thick; i++ {
			for j := -thick; j <= thick; j++ {
				if x+i >= 0 && x+i < size && y+j >= 0 && y+j < size {
					img.Set(x+i, y+j, col)
				}
			}
		}
	}

	centerX, centerY := 100, 128
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x-centerX), float64(y-centerY)
			dist := dx*dx + dy*dy
			if dist > 60*60 && dist < 82*82 && x < centerX+45 {
				dxIn, dyIn := float64(x-(centerX+35)), float64(y-centerY)
				if dxIn*dxIn+dyIn*dyIn > 65*65 {
					img.Set(x, y, col)
				}
			}
		}
	}
	for i := 0; i < 40; i++ {
		drawThickPoint(160+i, 70, 3)
		drawThickPoint(200-i, 70+i, 3)
		drawThickPoint(160+i, 110, 3)
	}
	for i := 0; i < 25; i++ {
		drawThickPoint(190+i, 130, 2)
		drawThickPoint(215-i, 130+i, 2)
		drawThickPoint(190+i, 155, 2)
	}

	f, err := os.Create(path)
	if err != nil { return err }
	defer f.Close()
	return png.Encode(f, img)
}

func loadConfig() {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	configPath = filepath.Join(exeDir, "config.json")
	iconPath = filepath.Join(exeDir, "icon.png")
	_ = generateIcon(iconPath)
	if data, err := ioutil.ReadFile(iconPath); err == nil {
		iconRes = fyne.NewStaticResource("icon.png", data)
	}
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		config = defaultConfig
		saveConfig()
		return
	}
	json.Unmarshal(data, &config)
}

func saveConfig() {
	data, _ := json.MarshalIndent(config, "", "  ")
	_ = ioutil.WriteFile(configPath, data, 0644)
}

func getLocalIP() string {
	addrs, _ := net.InterfaceAddrs()
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return "127.0.0.1"
}

func startServer() {
	serverMutex.Lock()
	defer serverMutex.Unlock()
	if serverInstance != nil { serverInstance.Close() }
	go func(port, path string) {
		exec.Command("fuser", "-k", port+"/tcp").Run()
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `<html><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><style>body{text-align:center;padding-top:50px;font-family:sans-serif;background-color:#f4f4f9;}.container{max-width:400px;margin:0 auto;padding:20px;background:white;border-radius:20px;box-shadow:0 4px 6px rgba(0,0,0,0.1);}.btn{width:100%%;height:100px;font-size:24px;background:#e74c3c;color:white;border:none;border-radius:15px;cursor:pointer;}</style><script>function doAction(){if(confirm("确定要立即休眠电脑吗？")){window.location.href="/do";}}</script></head><body><div class="container"><h2>Linux 远程控制</h2><button class="btn" onclick="doAction()">立即休眠电脑</button></div></body></html>`)
		})
		mux.HandleFunc("/do", func(w http.ResponseWriter, r *http.Request) {
			if time.Since(lastActionTime) < 3*time.Second { return }
			lastActionTime = time.Now()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, `<html><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><style>body{text-align:center;padding-top:50px;font-family:sans-serif;background-color:#f4f4f9;}.container{max-width:400px;margin:0 auto;padding:20px;background:white;border-radius:20px;box-shadow:0 4px 6px rgba(0,0,0,0.1);}.success{color:#27ae60;font-size:48px;margin-bottom:20px;}.msg{font-size:20px;color:#7f8c8d;}</style></head><body><div class="container"><div class="success">✓</div><div class="msg">指令已发送，电脑即将休眠。</div></div><script>setTimeout(()=>{window.location.href="/";},500);</script></body></html>`)
			go func() { time.Sleep(1 * time.Second); exec.Command("sh", path).Run() }()
		})
		serverInstance = &http.Server{Addr: ":" + port, Handler: mux}
		statusLabel.SetText(fmt.Sprintf("监听: http://%s:%s", getLocalIP(), port))
		_ = serverInstance.ListenAndServe()
	}(config.Port, config.ScriptPath)
}

type grayButtonRenderer struct {
	fyne.WidgetRenderer
	rect *canvas.Rectangle
}
func (r *grayButtonRenderer) Layout(size fyne.Size) { r.WidgetRenderer.Layout(size); r.rect.Resize(size) }
func (r *grayButtonRenderer) Objects() []fyne.CanvasObject { return append([]fyne.CanvasObject{r.rect}, r.WidgetRenderer.Objects()...) }

type grayButton struct{ widget.Button }
func newGrayButton(label string, tapped func()) *grayButton {
	b := &grayButton{}; b.Text = label; b.OnTapped = tapped
	b.ExtendBaseWidget(b); return b
}
func (b *grayButton) CreateRenderer() fyne.WidgetRenderer {
	r := b.Button.CreateRenderer()
	rect := canvas.NewRectangle(color.NRGBA{80, 80, 80, 255})
	rect.CornerRadius = 3
	return &grayButtonRenderer{WidgetRenderer: r, rect: rect}
}

func main() {
	loadConfig()
	myApp = app.New()
	myWindow = myApp.NewWindow("Linux远程控制")
	myWindow.Resize(compactSize)
	myWindow.SetFixedSize(false) // 允许代码调整尺寸

	if iconRes != nil { myWindow.SetIcon(iconRes) }

	scriptEntry := widget.NewEntry()
	scriptEntry.SetText(config.ScriptPath)

	browseBtn := widget.NewButton("选择", func() {
		// 1. 弹出选择器前先扩大窗口
		myWindow.Resize(expandedSize)
		
		d := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			// 3. 对话框关闭回调：无论是选了文件还是取消，都恢复尺寸
			if err == nil && reader != nil {
				scriptEntry.SetText(reader.URI().Path())
			}
			myWindow.Resize(compactSize)
		}, myWindow)
		
		d.SetFilter(storage.NewExtensionFileFilter([]string{".sh"}))
		// 2. 显示对话框
		d.Show()
	})

	portEntry := widget.NewEntry()
	portEntry.SetText(config.Port)
	statusLabel = widget.NewLabel("准备就绪")
	statusLabel.Alignment = fyne.TextAlignCenter
	tipLabel := widget.NewLabel("")
	tipLabel.Alignment = fyne.TextAlignCenter

	scriptRow := container.NewBorder(nil, nil, nil, browseBtn, scriptEntry)
	form := widget.NewForm(
		widget.NewFormItem("脚本", scriptRow),
		widget.NewFormItem("端口", portEntry),
	)

	startBtn := newGrayButton("更新配置", func() {
		config.Port, config.ScriptPath = portEntry.Text, scriptEntry.Text
		saveConfig()
		startServer()
		tipLabel.SetText("✓ 已应用配置")
		go func() { time.Sleep(2 * time.Second); tipLabel.SetText("") }()
	})

	exitBtn := newGrayButton("退出程序", func() { myApp.Quit() })
	bottomButtons := container.New(layout.NewGridLayout(2), startBtn, exitBtn)
	
	content := container.NewVBox(form, statusLabel, tipLabel, bottomButtons)
	myWindow.SetContent(content)

	if desk, ok := myApp.(desktop.App); ok {
		mShow := fyne.NewMenuItem("显示窗口", func() { myWindow.Show() })
		mQuit := fyne.NewMenuItem("退出程序", func() { myApp.Quit() })
		desk.SetSystemTrayMenu(fyne.NewMenu("控制中心", mShow, mQuit))
		if iconRes != nil { desk.SetSystemTrayIcon(iconRes) }
	}

	myWindow.SetCloseIntercept(func() { myWindow.Hide() })
	go startServer()
	myWindow.ShowAndRun()
}
