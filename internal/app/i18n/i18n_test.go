package i18n

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLanguageLookupAndFiles(t *testing.T) {
	translator := NewTranslator()
	translator.content = map[string]string{"custom": "translated"}
	if translator.GetLangText("custom") != "translated" || translator.GetLangText("ProgramVersion") == "ProgramVersion" || translator.GetLangText("missing-id") != "missing-id" {
		t.Fatal("language lookup order failed")
	}
	directory := t.TempDir()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(workingDirectory) })
	if translator.LoadLang("does-not-exist", nil) {
		t.Fatal("missing language unexpectedly loaded")
	}
	if err := os.Mkdir("lang", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("lang", "ok.json"), []byte(`{"hello":"world"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if !translator.LoadLang("ok", nil) || translator.GetLangText("hello") != "world" {
		t.Fatal("valid language file did not load")
	}
	if err := os.WriteFile(filepath.Join("lang", "bad.json"), []byte(`{`), 0644); err != nil {
		t.Fatal(err)
	}
	if translator.LoadLang("bad", nil) {
		t.Fatal("invalid language file loaded")
	}
	_ = GetLangCode()
}
