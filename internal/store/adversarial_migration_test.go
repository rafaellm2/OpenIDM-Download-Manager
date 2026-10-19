package store_test

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openidm/internal/store"

	_ "modernc.org/sqlite"
)

func TestAdversarialPreExistingSQLiteMigration(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "legacy_preexisting.db")

	// 1. Cria manualmente o banco SQLite pré-existente simulando uma versão anterior do OpenIDM / TurboDM
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("falha ao criar banco cru: %v", err)
	}

	createSchema := `
	CREATE TABLE categories (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		slug TEXT NOT NULL,
		extensions_json TEXT NOT NULL,
		default_dir TEXT NOT NULL,
		icon TEXT NOT NULL
	);
	CREATE TABLE app_settings (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		settings_json TEXT NOT NULL
	);
	`
	if _, err := rawDB.Exec(createSchema); err != nil {
		rawDB.Close()
		t.Fatalf("falha ao criar schema legado: %v", err)
	}

	// Insere categorias antigas com caminhos legados do Windows e Unix
	legacyCategories := []struct {
		id         string
		name       string
		defaultDir string
	}{
		{"docs", "Documentos", `C:\Users\Tester\Downloads\OpenIDM\Documentos`},
		{"music", "Música", `C:\Users\Tester\Downloads\OpenIDM\Musica`},
		{"video", "Vídeo", `/home/tester/Downloads/OpenIDM/Video`},
		{"turbodm", "Legado TurboDM", `C:\Users\Tester\Downloads\TurboDM\Programas`},
		{"root_win", "Root Win", `C:\Users\Tester\Downloads\OpenIDM`},
		{"root_win_slash", "Root Win Slash", `C:\Users\Tester\Downloads\OpenIDM\`},
		{"custom_safe", "Custom Safe", `D:\MyFiles\OpenIDM\Other`}, // Não está em Downloads, deve ser preservado
	}

	for _, c := range legacyCategories {
		_, err := rawDB.Exec(`
			INSERT INTO categories (id, name, slug, extensions_json, default_dir, icon)
			VALUES (?, ?, ?, '[]', ?, 'folder')`,
			c.id, c.name, c.id, c.defaultDir,
		)
		if err != nil {
			rawDB.Close()
			t.Fatalf("erro ao inserir categoria %s: %v", c.id, err)
		}
	}

	// Insere app_settings legado
	legacySettings := map[string]interface{}{
		"maxConnectionsPerDownload": 8,
		"defaultSaveDir":            `C:\Users\Tester\Downloads\OpenIDM`,
	}
	settingsBytes, _ := json.Marshal(legacySettings)
	_, err = rawDB.Exec(`INSERT INTO app_settings (id, settings_json) VALUES (1, ?)`, string(settingsBytes))
	if err != nil {
		rawDB.Close()
		t.Fatalf("erro ao inserir app_settings legado: %v", err)
	}

	// Fecha conexão crua antes de abrir via store.NewStore
	if err := rawDB.Close(); err != nil {
		t.Fatalf("falha ao fechar rawDB: %v", err)
	}

	// 2. Inicializa o Store do aplicativo no banco pré-existente
	st, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore falhou ao abrir e migrar banco legado: %v", err)
	}
	defer st.Close()

	// 3. Valida a reescrita dos caminhos em categories
	cats, err := st.ListCategories()
	if err != nil {
		t.Fatalf("ListCategories falhou: %v", err)
	}

	catMap := make(map[string]string)
	for _, c := range cats {
		catMap[c.ID] = c.DefaultDir
	}

	expectedMap := map[string]string{
		"docs":           `C:\Users\Tester\Downloads\Documentos`,
		"music":          `C:\Users\Tester\Downloads\Musica`,
		"video":          `/home/tester/Downloads/Video`,
		"turbodm":        `C:\Users\Tester\Downloads\Programas`,
		"root_win":       `C:\Users\Tester\Downloads`,
		"root_win_slash": `C:\Users\Tester\Downloads\`,
		"custom_safe":    `D:\MyFiles\OpenIDM\Other`,
	}

	for id, expected := range expectedMap {
		actual, exists := catMap[id]
		if !exists {
			t.Errorf("categoria %s não encontrada após migração", id)
			continue
		}
		if actual != expected {
			t.Errorf("migração da categoria %s incorreta: esperado %q, obtido %q", id, expected, actual)
		}
	}

	// 4. Valida a reescrita de app_settings
	settings, err := st.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings falhou: %v", err)
	}
	expectedSaveDir := `C:\Users\Tester\Downloads`
	if settings.DefaultSaveDir != expectedSaveDir {
		t.Errorf("DefaultSaveDir legado não foi migrado: esperado %q, obtido %q", expectedSaveDir, settings.DefaultSaveDir)
	}

	// 5. Teste de Idempotência: reabrir o Store não deve alterar caminhos nem corromper
	st.Close()
	st2, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("segunda abertura de NewStore falhou: %v", err)
	}
	defer st2.Close()

	settings2, err := st2.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings falhou após reabertura: %v", err)
	}
	if settings2.DefaultSaveDir != expectedSaveDir {
		t.Errorf("Idempotência falhou: esperado %q, obtido %q", expectedSaveDir, settings2.DefaultSaveDir)
	}
}

func TestAdversarialNewDatabaseDefaultPaths(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "fresh_database.db")

	st, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore falhou para banco novo: %v", err)
	}
	defer st.Close()

	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		homeDir = "."
	}
	expectedRoot := filepath.Join(homeDir, "Downloads")

	settings, err := st.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings falhou: %v", err)
	}

	if settings.DefaultSaveDir != expectedRoot {
		t.Errorf("Banco novo: DefaultSaveDir esperado %q, obtido %q", expectedRoot, settings.DefaultSaveDir)
	}

	if strings.Contains(settings.DefaultSaveDir, "OpenIDM") {
		t.Errorf("Banco novo: DefaultSaveDir contém 'OpenIDM': %s", settings.DefaultSaveDir)
	}

	cats, err := st.ListCategories()
	if err != nil {
		t.Fatalf("ListCategories falhou: %v", err)
	}

	for _, c := range cats {
		if strings.Contains(c.DefaultDir, "OpenIDM") {
			t.Errorf("Categoria %s contém 'OpenIDM': %s", c.ID, c.DefaultDir)
		}
		if !strings.HasPrefix(c.DefaultDir, expectedRoot) {
			t.Errorf("Categoria %s não começa com %s: %s", c.ID, expectedRoot, c.DefaultDir)
		}
	}
}
