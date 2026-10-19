package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"

	"openidm/internal/integration"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	startInTray := flag.Bool("tray", false, "Inicia o aplicativo oculto diretamente no canto do relógio (System Tray)")
	installerMode := flag.Bool("installer", false, "Abre o Assistente Profissional de Instalação e Extensão do OpenIDM")
	silentInstall := flag.Bool("install", false, "Executa a instalação completa do OpenIDM (auto-start, ícone, atalhos e extensão)")
	uninstallFlag := flag.Bool("uninstall", false, "Remove registros de inicialização automática, atalhos e Native Host")
	installDirFlag := flag.String("install-dir", "", "Diretório customizado de instalação para --install")
	launchExtFlag := flag.Bool("launch-ext", false, "Abre o navegador para ativar a extensão durante --install")
	dbPath := flag.String("db", "", "Caminho customizado para o banco SQLite")
	flag.Parse()

	if *uninstallFlag {
		if err := integration.RunUninstall(*installDirFlag); err != nil {
			fmt.Fprintf(os.Stderr, "Erro ao desinstalar OpenIDM: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("OpenIDM desinstalado com sucesso.")
		return
	}

	if *silentInstall {
		res, err := integration.RunInstallation(integration.InstallOptions{
			InstallDir:            *installDirFlag,
			StartWithWindows:      true,
			StartInTray:           true,
			CreateDesktopShortcut: true,
			CreateStartMenu:       true,
			RegisterUninstaller:   true,
			InstallBrowserExt:     true,
			LaunchBrowserExtSetup: *launchExtFlag,
			LaunchAppAfterInstall: false,
		}, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erro na instalação do OpenIDM: %v\n", err)
			os.Exit(1)
		}
		out, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(out))
		return
	}

	distFS, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		distFS = assets
	}

	app, err := NewApp(*dbPath, distFS)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erro ao inicializar OpenIDM: %v\n", err)
		os.Exit(1)
	}
	hasCLIUrl := false
	for _, arg := range os.Args[1:] {
		if isSupportedDownloadURL(arg) {
			hasCLIUrl = true
			break
		}
	}

	initWidth := 1240
	initHeight := 780
	if *installerMode {
		app.SetOpenInstallerOnStartup(true)
	} else if *startInTray || hasCLIUrl {
		app.SetInitialWindowMode("card")
		app.SetStartHidden(*startInTray)
		initWidth = DefaultCardWidth
		initHeight = DefaultCardPromptHeight
	}

	// Executa o aplicativo nativo do Windows (Wails v2 WebView2) com suporte ao Card Solto e bandeja do relógio.
	err = wails.Run(&options.App{
		Title:             AppWindowTitle,
		Width:             initWidth,
		Height:            initHeight,
		MinWidth:          500,
		MinHeight:         200,
		AlwaysOnTop:       false,
		StartHidden:       *startInTray,
		HideWindowOnClose: false, // Tratado via OnBeforeClose (app.BeforeClose -> HideToTray + notificação no relógio)
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 20, G: 20, B: 19, A: 1},
		OnStartup:        app.Startup,
		OnBeforeClose:    app.BeforeClose,
		OnShutdown:       app.Shutdown,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "com.rafatech.openidm.desktop.v1",
			OnSecondInstanceLaunch: func(secondInstanceData options.SecondInstanceData) {
				app.HandleSecondInstance(secondInstanceData.Args)
			},
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			Theme:                windows.Dark,
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Erro na janela nativa do OpenIDM: %v\n", err)
		os.Exit(1)
	}
}
