package integration

import (
	"context"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"openidm/internal/engine"
	"openidm/internal/store"

	"golang.org/x/sys/windows"
)

const cfUnicodeText = 13

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procOpenClip     = user32.NewProc("OpenClipboard")
	procCloseClip    = user32.NewProc("CloseClipboard")
	procEmptyClip    = user32.NewProc("EmptyClipboard")
	procGetClipData  = user32.NewProc("GetClipboardData")
	procSetClipData  = user32.NewProc("SetClipboardData")
	procGetClipSeq   = user32.NewProc("GetClipboardSequenceNumber")
	procGlobalAlloc  = kernel32.NewProc("GlobalAlloc")
	procGlobalLock   = kernel32.NewProc("GlobalLock")
	procGlobalUnlock = kernel32.NewProc("GlobalUnlock")
	procGlobalSize   = kernel32.NewProc("GlobalSize")
	procMoveMemory   = kernel32.NewProc("RtlMoveMemory")
)

// ClipboardCallback é acionado quando uma URL de arquivo monitorada é detectada na área de transferência.
type ClipboardCallback func(detectedURL string)

// ClipboardMonitor observa a área de transferência do sistema para captura automática de URLs.
type ClipboardMonitor struct {
	store      *store.Store
	callback   ClipboardCallback
	lastText   string
	lastSeq    uint32
	lastFireAt time.Time
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewClipboardMonitor cria um monitor de clipboard integrado às configurações do banco.
func NewClipboardMonitor(st *store.Store, cb ClipboardCallback) *ClipboardMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	// Inicializa lastText e lastSeq com o estado atual para não disparar falso-positivo ao abrir o app.
	raw, _ := readWindowsClipboard()
	initial := cleanClipboardCandidate(raw)
	return &ClipboardMonitor{
		store:    st,
		callback: cb,
		lastText: initial,
		lastSeq:  getClipboardSequenceNumber(),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start inicia o loop de verificação do clipboard em background.
func (c *ClipboardMonitor) Start() {
	if runtime.GOOS != "windows" {
		return
	}
	go func() {
		ticker := time.NewTicker(450 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
				c.check()
			}
		}
	}()
}

// Stop interrompe o monitoramento do clipboard.
func (c *ClipboardMonitor) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

func (c *ClipboardMonitor) check() {
	settings, err := c.store.GetSettings()
	if err != nil || !settings.MonitorClipboard {
		return
	}

	seq := getClipboardSequenceNumber()
	if seq != 0 && seq == c.lastSeq {
		return
	}

	raw, opened := readWindowsClipboard()
	if !opened && seq != 0 {
		// Se o clipboard estiver momentaneamente bloqueado por outro processo, tenta novamente no próximo tick.
		return
	}
	txt := cleanClipboardCandidate(raw)
	if txt == "" || len(txt) > 2048 {
		c.lastSeq = seq
		return
	}

	// Se o número de sequência mudou (usuário deu Ctrl+C novamente), permite recapturar o mesmo link após 1.5s.
	if txt == c.lastText && (seq == 0 || time.Since(c.lastFireAt) < 1500*time.Millisecond) {
		c.lastSeq = seq
		return
	}

	c.lastSeq = seq
	c.lastText = txt

	normalized := engine.NormalizeDownloadURL(txt)
	rules, _ := c.store.ListSiteRules()
	if ok, _ := ShouldCaptureURL(normalized, "", settings, rules); ok {
		c.lastFireAt = time.Now()
		if c.callback != nil {
			c.callback(normalized)
		}
	}
}

func cleanClipboardCandidate(s string) string {
	s = strings.TrimSpace(s)
	// Se houver múltiplas linhas, considera a primeira linha não vazia.
	if idx := strings.IndexAny(s, "\r\n"); idx >= 0 {
		s = strings.TrimSpace(s[:idx])
	}
	return strings.Trim(s, "\"'<>")
}

func getClipboardSequenceNumber() uint32 {
	if runtime.GOOS != "windows" {
		return 0
	}
	r, _, _ := procGetClipSeq.Call()
	return uint32(r)
}

// ReadClipboardText retorna o texto atual copiado na área de transferência do Windows.
func ReadClipboardText() string {
	raw, _ := readWindowsClipboard()
	return cleanClipboardCandidate(raw)
}

// readWindowsClipboard lê texto Unicode diretamente via Win32 API sem criar processos externos.
// Retorna (texto, opened) onde opened=false indica que outro processo detém o lock da área de transferência.
func readWindowsClipboard() (string, bool) {
	if runtime.GOOS != "windows" {
		return "", true
	}
	r, _, _ := procOpenClip.Call(0)
	if r == 0 {
		return "", false
	}
	defer procCloseClip.Call()

	h, _, _ := procGetClipData.Call(cfUnicodeText)
	if h == 0 {
		return "", true
	}

	sz, _, _ := procGlobalSize.Call(h)
	if sz < 2 {
		return "", true
	}
	if sz > 16384 {
		sz = 16384
	}

	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		return "", true
	}
	defer procGlobalUnlock.Call(h)

	buf := make([]uint16, sz/2)
	_, _, _ = procMoveMemory.Call(uintptr(unsafe.Pointer(&buf[0])), ptr, sz)
	return windows.UTF16ToString(buf), true
}

// WriteClipboardText grava texto Unicode diretamente na área de transferência do Windows via Win32 API.
func WriteClipboardText(text string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	u16, err := windows.UTF16FromString(text)
	if err != nil {
		return err
	}
	r, _, _ := procOpenClip.Call(0)
	if r == 0 {
		return syscall.EINVAL
	}
	defer procCloseClip.Call()

	_, _, _ = procEmptyClip.Call()
	sizeBytes := uintptr(len(u16) * 2)
	const gmemMoveable = 0x0002
	hMem, _, _ := procGlobalAlloc.Call(gmemMoveable, sizeBytes)
	if hMem == 0 {
		return syscall.ENOMEM
	}
	ptr, _, _ := procGlobalLock.Call(hMem)
	if ptr == 0 {
		return syscall.ENOMEM
	}
	_, _, _ = procMoveMemory.Call(ptr, uintptr(unsafe.Pointer(&u16[0])), sizeBytes)
	_, _, _ = procGlobalUnlock.Call(hMem)
	_, _, _ = procSetClipData.Call(cfUnicodeText, hMem)
	return nil
}

