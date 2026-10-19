package integration

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"openidm/internal/models"

	"golang.org/x/sys/windows"
)

const (
	wmDestroy           = 0x0002
	wmClose             = 0x0010
	wmCommand           = 0x0111
	wmLButtonUp         = 0x0202
	wmLButtonDblClk     = 0x0203
	wmRButtonUp         = 0x0205
	wmContextMenu       = 0x007B
	wmApp               = 0x8000
	wmTrayCallback      = wmApp + 42
	ninBalloonUserClick = 0x0405 // WM_USER (0x0400) + 5

	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	nifInfo    = 0x00000010

	niifInfo = 0x00000001

	mfString       = 0x00000000
	mfSeparator    = 0x00000800
	tpmRightButton = 0x0002
	tpmBottomAlign = 0x0020
	tpmNonotify    = 0x0080
	tpmReturnCmd   = 0x0100

	swRestore = 9
	swShow    = 5

	swpNoSize     = 0x0001
	swpNoMove     = 0x0002
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010
	swpShowWindow = 0x0040

	trayCmdOpen         = 1001
	trayCmdCaptureLink  = 1002
	trayCmdStartQueue   = 1003
	trayCmdPauseAll     = 1004
	trayCmdQuit         = 1005
	trayCmdOpenCard     = 1006
)

var (
	shell32                      = syscall.NewLazyDLL("shell32.dll")
	procShellNotifyIconW         = shell32.NewProc("Shell_NotifyIconW")
	procRegisterClassExW         = user32.NewProc("RegisterClassExW")
	procCreateWindowExW          = user32.NewProc("CreateWindowExW")
	procDefWindowProcW           = user32.NewProc("DefWindowProcW")
	procDestroyWindow            = user32.NewProc("DestroyWindow")
	procGetMessageW              = user32.NewProc("GetMessageW")
	procTranslateMessage         = user32.NewProc("TranslateMessage")
	procDispatchMessageW         = user32.NewProc("DispatchMessageW")
	procPostQuitMessage          = user32.NewProc("PostQuitMessage")
	procPostMessageW             = user32.NewProc("PostMessageW")
	procCreatePopupMenu          = user32.NewProc("CreatePopupMenu")
	procAppendMenuW              = user32.NewProc("AppendMenuW")
	procSetMenuDefaultItem       = user32.NewProc("SetMenuDefaultItem")
	procTrackPopupMenu           = user32.NewProc("TrackPopupMenu")
	procDestroyMenu              = user32.NewProc("DestroyMenu")
	procGetCursorPos             = user32.NewProc("GetCursorPos")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procAttachThreadInput        = user32.NewProc("AttachThreadInput")
	procBringWindowToTop         = user32.NewProc("BringWindowToTop")
	procSetWindowPos             = user32.NewProc("SetWindowPos")
	procGetWindowRect            = user32.NewProc("GetWindowRect")
	procGetClientRect            = user32.NewProc("GetClientRect")
	procGetDpiForWindow          = user32.NewProc("GetDpiForWindow")
	procGetWindowLongW           = user32.NewProc("GetWindowLongW")
	procSetActiveWindow          = user32.NewProc("SetActiveWindow")
	procSetFocus                 = user32.NewProc("SetFocus")
	procSwitchToThisWindow       = user32.NewProc("SwitchToThisWindow")
	procKeybdEvent               = user32.NewProc("keybd_event")
	procFindWindowW              = user32.NewProc("FindWindowW")
	procShowWindow               = user32.NewProc("ShowWindow")
	procIsIconic                 = user32.NewProc("IsIconic")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procSystemParametersInfoW    = user32.NewProc("SystemParametersInfoW")
	procCreateIcon               = user32.NewProc("CreateIcon")
	procDestroyIcon              = user32.NewProc("DestroyIcon")
	procLoadIconW                = user32.NewProc("LoadIconW")
	procSendMessageW             = user32.NewProc("SendMessageW")
	procMonitorFromWindow        = user32.NewProc("MonitorFromWindow")
	procMonitorFromRect          = user32.NewProc("MonitorFromRect")
	procGetMonitorInfoW          = user32.NewProc("GetMonitorInfoW")
	procGetModuleHandleW         = kernel32.NewProc("GetModuleHandleW")
	procGetCurrentThreadId       = kernel32.NewProc("GetCurrentThreadId")
)

// notifyIconDataW mapeia a estrutura NOTIFYICONDATAW da API Win32 (shellapi.h).
type notifyIconDataW struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type winPoint struct {
	X int32
	Y int32
}

type winRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type winMsg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      winPoint
}

type winMonitorInfo struct {
	CbSize    uint32
	RcMonitor winRect
	RcWork    winRect
	DwFlags   uint32
}

// TrayCallbacks reúne os gatilhos acionados pelo ícone na bandeja do sistema (canto do relógio).
type TrayCallbacks struct {
	OnOpen            func()
	OnOpenCard        func()
	OnCaptureLink     func()
	OnStartMainQueue  func()
	OnPauseAll        func()
	OnQuit            func()
}

// SystemTray gerencia o ícone nativo do OpenIDM na área de notificação do Windows (System Tray).
type SystemTray struct {
	mu        sync.Mutex
	callbacks TrayCallbacks
	language  string
	hwnd      uintptr
	hIcon     uintptr
	nid       notifyIconDataW
	started   bool
	stopped   bool
}

var activeTrayInstance *SystemTray

// NewSystemTray cria o controlador da bandeja do sistema do Windows.
func NewSystemTray(cbs TrayCallbacks) *SystemTray {
	return &SystemTray{
		callbacks: cbs,
		language:  "pt-BR",
	}
}

func trayTooltipForLang(lang string) string {
	if models.NormalizeLanguage(lang) == "en-US" {
		return "OpenIDM — Download Manager (Click to open)"
	}
	return "OpenIDM — Gerenciador de Downloads (Clique para abrir)"
}

// SetLanguage define o idioma do menu e tooltip da bandeja do sistema ("pt-BR" ou "en-US").
func (t *SystemTray) SetLanguage(lang string) {
	norm := models.NormalizeLanguage(lang)
	t.mu.Lock()
	t.language = norm
	hwnd := t.hwnd
	stopped := t.stopped
	if hwnd != 0 && !stopped {
		copyUTF16Fixed(t.nid.SzTip[:], trayTooltipForLang(norm))
		t.nid.UFlags = nifMessage | nifIcon | nifTip
		if runtime.GOOS == "windows" {
			_, _, _ = procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&t.nid)))
		}
	}
	t.mu.Unlock()
}

// GetLanguage retorna o idioma atualmente ativo na bandeja do sistema.
func (t *SystemTray) GetLanguage() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return models.NormalizeLanguage(t.language)
}

// Start inicializa a thread dedicada de mensagens Win32 e adiciona o ícone no canto do relógio.
func (t *SystemTray) Start() {
	if runtime.GOOS != "windows" {
		return
	}
	t.mu.Lock()
	if t.started {
		t.mu.Unlock()
		return
	}
	t.started = true
	activeTrayInstance = t
	t.mu.Unlock()

	go t.runLoop()
}

// Stop remove o ícone da bandeja do sistema e encerra o loop de mensagens.
func (t *SystemTray) Stop() {
	if runtime.GOOS != "windows" {
		return
	}
	t.mu.Lock()
	if !t.started || t.stopped {
		t.mu.Unlock()
		return
	}
	t.stopped = true
	hwnd := t.hwnd
	nid := t.nid
	hIcon := t.hIcon
	t.mu.Unlock()

	if nid.HWnd != 0 {
		_, _, _ = procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	}
	if hIcon != 0 {
		_, _, _ = procDestroyIcon.Call(hIcon)
	}
	if hwnd != 0 {
		_, _, _ = procPostMessageW.Call(hwnd, wmClose, 0, 0)
	}
}

// UpdateTooltip atualiza o texto exibido ao passar o mouse sobre o ícone no canto do relógio.
func (t *SystemTray) UpdateTooltip(tip string) {
	if runtime.GOOS != "windows" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.hwnd == 0 || t.stopped {
		return
	}
	copyUTF16Fixed(t.nid.SzTip[:], tip)
	t.nid.UFlags = nifMessage | nifIcon | nifTip
	_, _, _ = procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&t.nid)))
}

// ShowBalloon exibe uma notificação nativa do Windows a partir do ícone da bandeja.
func (t *SystemTray) ShowBalloon(title, message string) {
	if runtime.GOOS != "windows" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.hwnd == 0 || t.stopped {
		return
	}
	copyUTF16Fixed(t.nid.SzInfoTitle[:], title)
	copyUTF16Fixed(t.nid.SzInfo[:], message)
	t.nid.UFlags = nifMessage | nifIcon | nifTip | nifInfo
	t.nid.DwInfoFlags = niifInfo
	_, _, _ = procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&t.nid)))
}

func (t *SystemTray) runLoop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := windows.UTF16PtrFromString("OpenIDMTrayWindowClass")

	wc := wndClassExW{
		LpfnWndProc:   syscall.NewCallback(trayWndProc),
		HInstance:     hInst,
		LpszClassName: className,
	}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	_, _, _ = procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	winTitle, _ := windows.UTF16PtrFromString("OpenIDMTrayHiddenWindow")
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(winTitle)),
		0,
		0, 0, 0, 0,
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		return
	}

	hIcon := createOpenIDMIcon()
	if hIcon == 0 {
		hIcon, _, _ = procLoadIconW.Call(0, 32512) // IDI_APPLICATION fallback
	}

	t.mu.Lock()
	t.hwnd = hwnd
	t.hIcon = hIcon
	t.nid = notifyIconDataW{
		HWnd:             hwnd,
		UID:              1,
		UFlags:           nifMessage | nifIcon | nifTip,
		UCallbackMessage: wmTrayCallback,
		HIcon:            hIcon,
	}
	t.nid.CbSize = uint32(unsafe.Sizeof(t.nid))
	copyUTF16Fixed(t.nid.SzTip[:], trayTooltipForLang(t.language))
	nidCopy := t.nid
	t.mu.Unlock()

	_, _, _ = procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nidCopy)))

	var msg winMsg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func trayWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	t := activeTrayInstance
	switch msg {
	case wmTrayCallback:
		event := uint32(lParam) & 0xFFFF
		switch event {
		case wmLButtonUp, wmLButtonDblClk, ninBalloonUserClick:
			if t != nil && t.callbacks.OnOpen != nil {
				go t.callbacks.OnOpen()
			}
			return 0
		case wmRButtonUp, wmContextMenu:
			if t != nil {
				t.showContextMenu(hwnd)
			}
			return 0
		}

	case wmClose:
		_, _, _ = procDestroyWindow.Call(hwnd)
		return 0

	case wmDestroy:
		_, _, _ = procPostQuitMessage.Call(0)
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

func (t *SystemTray) showContextMenu(hwnd uintptr) {
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hMenu)

	isEN := t.GetLanguage() == "en-US"
	if isEN {
		appendMenuText(hMenu, trayCmdOpen, "🚀 Open OpenIDM (Full App)")
		appendMenuText(hMenu, trayCmdOpenCard, "🗂 Open Compact Card (Downloads List)")
		appendMenuText(hMenu, trayCmdCaptureLink, "➕ New Download (Save Link)...")
		_, _, _ = procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
		appendMenuText(hMenu, trayCmdStartQueue, "⚡ Start Main Queue")
		appendMenuText(hMenu, trayCmdPauseAll, "⏸ Pause All Downloads")
		_, _, _ = procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
		appendMenuText(hMenu, trayCmdQuit, "❌ Quit OpenIDM")
	} else {
		appendMenuText(hMenu, trayCmdOpen, "🚀 Abrir OpenIDM (App Completo)")
		appendMenuText(hMenu, trayCmdOpenCard, "🗂 Abrir Card Reduzido (Lista de Downloads)")
		appendMenuText(hMenu, trayCmdCaptureLink, "➕ Novo Download (Salvar Link)...")
		_, _, _ = procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
		appendMenuText(hMenu, trayCmdStartQueue, "⚡ Iniciar Fila Principal")
		appendMenuText(hMenu, trayCmdPauseAll, "⏸ Pausar Todos os Downloads")
		_, _, _ = procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
		appendMenuText(hMenu, trayCmdQuit, "❌ Sair do OpenIDM")
	}

	// Destaca o App Completo como opção padrão no ícone do relógio.
	_, _, _ = procSetMenuDefaultItem.Call(hMenu, trayCmdOpen, 0)

	var pt winPoint
	_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	_, _, _ = procSetForegroundWindow.Call(hwnd)

	flags := uintptr(tpmRightButton | tpmBottomAlign | tpmNonotify | tpmReturnCmd)
	cmd, _, _ := procTrackPopupMenu.Call(hMenu, flags, uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0)
	_, _, _ = procPostMessageW.Call(hwnd, 0, 0, 0)

	switch uint32(cmd) {
	case trayCmdOpen:
		if t.callbacks.OnOpen != nil {
			go t.callbacks.OnOpen()
		}
	case trayCmdOpenCard:
		if t.callbacks.OnOpenCard != nil {
			go t.callbacks.OnOpenCard()
		}
	case trayCmdCaptureLink:
		if t.callbacks.OnCaptureLink != nil {
			go t.callbacks.OnCaptureLink()
		}
	case trayCmdStartQueue:
		if t.callbacks.OnStartMainQueue != nil {
			go t.callbacks.OnStartMainQueue()
		}
	case trayCmdPauseAll:
		if t.callbacks.OnPauseAll != nil {
			go t.callbacks.OnPauseAll()
		}
	case trayCmdQuit:
		if t.callbacks.OnQuit != nil {
			go t.callbacks.OnQuit()
		}
	}
}

func appendMenuText(hMenu uintptr, id uintptr, label string) {
	ptr, err := windows.UTF16PtrFromString(label)
	if err != nil {
		return
	}
	_, _, _ = procAppendMenuW.Call(hMenu, mfString, id, uintptr(unsafe.Pointer(ptr)))
}

func copyUTF16Fixed(dst []uint16, s string) {
	for i := range dst {
		dst[i] = 0
	}
	u16, err := windows.UTF16FromString(s)
	if err != nil {
		return
	}
	if len(u16) >= len(dst) {
		copy(dst, u16[:len(dst)-1])
		dst[len(dst)-1] = 0
		return
	}
	copy(dst, u16)
}

// createOpenIDMIcon cria em memória o ícone nativo 32x32 ARGB oficial do OpenIDM para a bandeja do sistema.
func createOpenIDMIcon() uintptr {
	return createOpenIDMIconSize(32)
}

func createOpenIDMIconSize(size int) uintptr {
	if size <= 0 {
		size = 32
	}
	img := RenderOpenIDMIcon(size)
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()
	andRowBytes := ((width + 31) / 32) * 4
	andMask := make([]byte, andRowBytes*height)
	xorPixels := make([]byte, width*height*4) // BGRA top-down para CreateIcon

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := img.NRGBAAt(x, y)
			idx := (y*width + x) * 4
			// Premultiply alpha para renderização suave na bandeja e barra de tarefas
			a := uint16(c.A)
			xorPixels[idx+0] = uint8((uint16(c.B) * a) / 255)
			xorPixels[idx+1] = uint8((uint16(c.G) * a) / 255)
			xorPixels[idx+2] = uint8((uint16(c.R) * a) / 255)
			xorPixels[idx+3] = c.A
			if c.A < 8 {
				byteIdx := y*andRowBytes + (x / 8)
				bitIdx := uint(7 - (x % 8))
				andMask[byteIdx] |= 1 << bitIdx
			}
		}
	}

	hIcon, _, _ := procCreateIcon.Call(
		0,
		uintptr(width),
		uintptr(height),
		1,
		32,
		uintptr(unsafe.Pointer(&andMask[0])),
		uintptr(unsafe.Pointer(&xorPixels[0])),
	)
	return hIcon
}

// ApplyWindowIcon aplica o ícone oficial do OpenIDM (pequeno 32x32 e grande 64x64) na janela nativa Win32 (barra de título e barra de tarefas).
func ApplyWindowIcon(windowTitle string) {
	if runtime.GOOS != "windows" {
		return
	}
	hwnd := findAppWindowHandle(windowTitle)
	if hwnd == 0 {
		return
	}
	const (
		wmSetIcon = 0x0080
		iconSmall = 0
		iconBig   = 1
	)
	if small := createOpenIDMIconSize(32); small != 0 {
		_, _, _ = procSendMessageW.Call(hwnd, wmSetIcon, iconSmall, small)
	}
	if big := createOpenIDMIconSize(64); big != 0 {
		_, _, _ = procSendMessageW.Call(hwnd, wmSetIcon, iconBig, big)
	}
}

func findAppWindowHandle(windowTitle string) uintptr {
	var hwnd uintptr
	if windowTitle != "" {
		if titlePtr, err := windows.UTF16PtrFromString(windowTitle); err == nil {
			hwnd, _, _ = procFindWindowW.Call(0, uintptr(unsafe.Pointer(titlePtr)))
		}
	}
	if hwnd == 0 {
		if clsPtr, err := windows.UTF16PtrFromString("wailsWindow"); err == nil {
			hwnd, _, _ = procFindWindowW.Call(uintptr(unsafe.Pointer(clsPtr)), 0)
		}
	}
	return hwnd
}

// GetForegroundWindowTitle retorna o título da janela atualmente em primeiro plano no Windows.
func GetForegroundWindowTitle() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	fgHwnd, _, _ := procGetForegroundWindow.Call()
	if fgHwnd == 0 {
		return ""
	}
	buf := make([]uint16, 256)
	n, _, _ := procGetWindowTextW.Call(fgHwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// ForceForegroundWindow restaura e traz a janela nativa do aplicativo para a frente no Windows,
// contornando o bloqueio de foco em segundo plano (ForegroundLockTimeout) do Windows 10/11.
func ForceForegroundWindow(windowTitle string) {
	if runtime.GOOS != "windows" {
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hwnd := findAppWindowHandle(windowTitle)
	if hwnd == 0 {
		return
	}

	iconic, _, _ := procIsIconic.Call(hwnd)
	visible, _, _ := procIsWindowVisible.Call(hwnd)
	if iconic != 0 || visible == 0 {
		_, _, _ = procShowWindow.Call(hwnd, swRestore)
	}
	_, _, _ = procShowWindow.Call(hwnd, swShow)

	fgHwnd, _, _ := procGetForegroundWindow.Call()
	var fgThread uintptr
	if fgHwnd != 0 && fgHwnd != hwnd {
		fgThread, _, _ = procGetWindowThreadProcessId.Call(fgHwnd, 0)
	}
	targetThread, _, _ := procGetWindowThreadProcessId.Call(hwnd, 0)
	curThread, _, _ := procGetCurrentThreadId.Call()

	attachedFgTarget := false
	attachedFgCur := false
	attachedCurTarget := false

	if fgThread != 0 && targetThread != 0 && fgThread != targetThread {
		ret, _, _ := procAttachThreadInput.Call(fgThread, targetThread, 1)
		attachedFgTarget = ret != 0
	}
	if fgThread != 0 && curThread != 0 && fgThread != curThread {
		ret, _, _ := procAttachThreadInput.Call(fgThread, curThread, 1)
		attachedFgCur = ret != 0
	}
	if curThread != 0 && targetThread != 0 && curThread != targetThread {
		ret, _, _ := procAttachThreadInput.Call(curThread, targetThread, 1)
		attachedCurTarget = ret != 0
	}

	// Libera a trava de foco do kernel Win32 (SPI_SETFOREGROUNDLOCKTIMEOUT + VK_MENU keyup), traz para frente e libera HWND_NOTOPMOST.
	const spiSetForegroundLockTimeout = 0x2001
	_, _, _ = procSystemParametersInfoW.Call(spiSetForegroundLockTimeout, 0, 0, 0)
	_, _, _ = procKeybdEvent.Call(0x12, 0, 0x0002, 0)
	hwndTopMost := ^uintptr(0)   // HWND_TOPMOST (-1) temporário apenas para destravar foco
	hwndNoTopMost := ^uintptr(1) // HWND_NOTOPMOST (-2) permite ficar atrás de outras janelas normalmente
	_, _, _ = procSetWindowPos.Call(hwnd, hwndTopMost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpShowWindow)
	_, _, _ = procBringWindowToTop.Call(hwnd)
	_, _, _ = procSetForegroundWindow.Call(hwnd)
	_, _, _ = procSwitchToThisWindow.Call(hwnd, 1)
	_, _, _ = procSetActiveWindow.Call(hwnd)
	_, _, _ = procSetFocus.Call(hwnd)
	_, _, _ = procSetWindowPos.Call(hwnd, hwndNoTopMost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpShowWindow)

	if attachedCurTarget {
		_, _, _ = procAttachThreadInput.Call(curThread, targetThread, 0)
	}
	if attachedFgCur {
		_, _, _ = procAttachThreadInput.Call(fgThread, curThread, 0)
	}
	if attachedFgTarget {
		_, _, _ = procAttachThreadInput.Call(fgThread, targetThread, 0)
	}
}

// ReleaseTopMostWindow remove o flag WS_EX_TOPMOST caso presente, garantindo que a janela possa ficar atrás de outras janelas
// sem alterar a ordem Z nem puxar a janela para frente quando ela já estiver atrás de outra janela normal.
func ReleaseTopMostWindow(windowTitle string) {
	if runtime.GOOS != "windows" {
		return
	}
	hwnd := findAppWindowHandle(windowTitle)
	if hwnd == 0 {
		return
	}
	const wsExTopMost = 0x00000008
	gwlExStyle := ^uintptr(19) // GWL_EXSTYLE (-20)
	exStyle, _, _ := procGetWindowLongW.Call(hwnd, gwlExStyle)
	if (exStyle & wsExTopMost) != 0 {
		hwndNoTopMost := ^uintptr(1) // HWND_NOTOPMOST (-2)
		_, _, _ = procSetWindowPos.Call(hwnd, hwndNoTopMost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
	}
}

// getWindowScaleAndFrame calcula a escala DPI da janela e a espessura exata da moldura/barra de título nativa Win32.
func getWindowScaleAndFrame(hwnd uintptr) (scale float64, frameW int, frameH int) {
	dpi := 96
	if procGetDpiForWindow.Find() == nil {
		if d, _, _ := procGetDpiForWindow.Call(hwnd); d >= 96 && d <= 480 {
			dpi = int(d)
		}
	}
	scale = float64(dpi) / 96.0
	frameW = int(16*scale + 0.5)
	frameH = int(39*scale + 0.5)

	var wRect, cRect winRect
	wOk, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wRect)))
	cOk, _, _ := procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&cRect)))
	if wOk != 0 && cOk != 0 && wRect.Right > wRect.Left && cRect.Right > 0 && cRect.Bottom > 0 {
		fw := int((wRect.Right - wRect.Left) - (cRect.Right - cRect.Left))
		fh := int((wRect.Bottom - wRect.Top) - (cRect.Bottom - cRect.Top))
		if fw >= 0 && fw <= int(48*scale) && fh >= int(20*scale) && fh <= int(96*scale) {
			frameW = fw
			frameH = fh
		}
	}
	return scale, frameW, frameH
}

// GetWindowBounds retorna a posição (X, Y) e dimensões (Width, Height) atuais da janela nativa caso esteja visível e não minimizada.
func GetWindowBounds(windowTitle string) (x, y, width, height int, ok bool) {
	if runtime.GOOS != "windows" {
		return 0, 0, 0, 0, false
	}
	hwnd := findAppWindowHandle(windowTitle)
	if hwnd == 0 {
		return 0, 0, 0, 0, false
	}
	visible, _, _ := procIsWindowVisible.Call(hwnd)
	iconic, _, _ := procIsIconic.Call(hwnd)
	if visible == 0 || iconic != 0 {
		return 0, 0, 0, 0, false
	}
	var r winRect
	ret, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	if ret == 0 || r.Right <= r.Left || r.Bottom <= r.Top {
		return 0, 0, 0, 0, false
	}
	return int(r.Left), int(r.Top), int(r.Right - r.Left), int(r.Bottom - r.Top), true
}

// ApplyStandaloneCardWindow dimensiona e posiciona a janela nativa como um card retangular no canto esquerdo da tela.
func ApplyStandaloneCardWindow(windowTitle string, width, height int, reposition bool) {
	_, _, _ = ApplyStandaloneCardWindowAt(windowTitle, width, height, reposition, 0, 0, false)
}

// ApplyStandaloneCardWindowAt dimensiona e posiciona o card retangular no canto esquerdo da tela por padrão,
// convertendo pixels lógicos CSS para pixels físicos com compensação de DPI e barra de título Win32,
// respeitando a preferência de posição salva pelo usuário (inclusive multi-monitor) e permitindo ficar atrás de outras janelas.
func ApplyStandaloneCardWindowAt(
	windowTitle string,
	width, height int,
	reposition bool,
	savedX, savedY int,
	hasSavedPos bool,
) (appliedX, appliedY int, ok bool) {
	if runtime.GOOS != "windows" {
		return savedX, savedY, false
	}
	hwnd := findAppWindowHandle(windowTitle)
	if hwnd == 0 {
		return savedX, savedY, false
	}

	visible, _, _ := procIsWindowVisible.Call(hwnd)
	iconic, _, _ := procIsIconic.Call(hwnd)
	if !reposition && visible == 0 {
		return savedX, savedY, false
	}

	if width < 500 {
		width = 700
	}
	if height < 175 {
		height = 240
	}

	scale, frameW, frameH := getWindowScaleAndFrame(hwnd)
	physW := int(float64(width)*scale+0.5) + frameW
	physH := int(float64(height)*scale+0.5) + frameH

	// Obtém a área útil da tela principal descontando a barra de tarefas (SPI_GETWORKAREA = 0x0030)
	const spiGetWorkArea = 0x0030
	var workArea winRect
	_, _, _ = procSystemParametersInfoW.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&workArea)), 0)
	if workArea.Right <= workArea.Left || workArea.Bottom <= workArea.Top {
		workArea = winRect{Left: 0, Top: 0, Right: 1920, Bottom: 1040}
	}

	const monitorDefaultToNearest = 2
	if hasSavedPos && procMonitorFromRect.Find() == nil {
		// Localiza o monitor correspondente às coordenadas salvas (ou o monitor ativo mais próximo caso desconectado)
		targetRect := winRect{
			Left:   int32(savedX),
			Top:    int32(savedY),
			Right:  int32(savedX + physW),
			Bottom: int32(savedY + physH),
		}
		if hMon, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&targetRect)), monitorDefaultToNearest); hMon != 0 {
			var mi winMonitorInfo
			mi.CbSize = uint32(unsafe.Sizeof(mi))
			if monOk, _, _ := procGetMonitorInfoW.Call(hMon, uintptr(unsafe.Pointer(&mi))); monOk != 0 {
				if mi.RcWork.Right > mi.RcWork.Left && mi.RcWork.Bottom > mi.RcWork.Top {
					workArea = mi.RcWork
				}
			}
		}
	} else if !reposition {
		if hMon, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultToNearest); hMon != 0 {
			var mi winMonitorInfo
			mi.CbSize = uint32(unsafe.Sizeof(mi))
			if monOk, _, _ := procGetMonitorInfoW.Call(hMon, uintptr(unsafe.Pointer(&mi))); monOk != 0 {
				if mi.RcWork.Right > mi.RcWork.Left && mi.RcWork.Bottom > mi.RcWork.Top {
					workArea = mi.RcWork
				}
			}
		}
	}

	maxW := int(workArea.Right-workArea.Left) - 24
	maxH := int(workArea.Bottom-workArea.Top) - 24
	if maxW > 500 && physW > maxW {
		physW = maxW
	}
	if maxH > 200 && physH > maxH {
		physH = maxH
	}

	var curRect winRect
	ret, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&curRect)))

	// Posição padrão: canto esquerdo da tela
	workH := int(workArea.Bottom - workArea.Top)
	defaultTopOffset := (workH - physH) / 3
	if defaultTopOffset < 48 {
		defaultTopOffset = 48
	}
	x := int(workArea.Left) + 20
	y := int(workArea.Top) + defaultTopOffset

	if hasSavedPos {
		x = savedX
		y = savedY
	}

	// Se o card já estava aberto e posicionado (redimensionamento dinâmico de altura),
	// preserva o canto superior esquerdo (Left, Top) atual para não pular na tela
	maxCardPhysW := int(1020*scale) + frameW
	if !reposition && ret != 0 && curRect.Right > curRect.Left && iconic == 0 && visible != 0 {
		curW := int(curRect.Right - curRect.Left)
		if curW <= maxCardPhysW {
			x = int(curRect.Left)
			y = int(curRect.Top)
		}
	}

	// Garante que o card nunca fique fora da área visível da tela
	if x+physW > int(workArea.Right)-8 {
		x = int(workArea.Right) - physW - 8
	}
	if x < int(workArea.Left)+8 {
		x = int(workArea.Left) + 8
	}
	if y+physH > int(workArea.Bottom)-12 {
		y = int(workArea.Bottom) - physH - 12
	}
	if y < int(workArea.Top)+8 {
		y = int(workArea.Top) + 8
	}

	flags := uintptr(swpShowWindow)
	if !reposition {
		// Ao redimensionar automaticamente, não altera Z-order nem rouba foco, permitindo ficar atrás de outras janelas
		flags = uintptr(swpNoZOrder | swpNoActivate)
	}

	hwndNoTopMost := ^uintptr(1) // HWND_NOTOPMOST (-2): permite ficar atrás de outras janelas normalmente
	_, _, _ = procSetWindowPos.Call(
		hwnd,
		hwndNoTopMost,
		uintptr(x),
		uintptr(y),
		uintptr(physW),
		uintptr(physH),
		flags,
	)

	var finalRect winRect
	if fRet, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&finalRect))); fRet != 0 && finalRect.Right > finalRect.Left {
		return int(finalRect.Left), int(finalRect.Top), true
	}

	return x, y, true
}




