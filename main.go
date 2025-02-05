package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/chromedp/chromedp"
)

func main() {
	// コマンドライン引数を定義
	pageURL := flag.String("url", "", "GROWIのページURL")
	outDir := flag.String("out", "", "画像保存先ディレクトリのパス")
	flag.Parse()

	// 引数チェック
	if *pageURL == "" || *outDir == "" {
		flag.Usage()
		os.Exit(1)
	}

	// 画像保存先ディレクトリを作成（存在しない場合）
	if err := os.MkdirAll(*outDir, 0755); err != nil {
		log.Fatalf("画像保存先ディレクトリの作成に失敗: %v", err)
	}

	// ベースとなるURLをパースしておく（相対パス解決用）
	base, err := url.Parse(*pageURL)
	if err != nil {
		log.Fatalf("ページURLのパースに失敗: %v", err)
	}

	// chromedp用のExecAllocatorオプションを生成
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		// 必要に応じてheadlessモードをオフにできる（デバッグ用）
		// chromedp.Flag("headless", false),
	)
	// カレントユーザのChromeプロファイルディレクトリを設定
	profileDir := getChromeProfileDir()
	if profileDir != "" {
		opts = append(opts, chromedp.Flag("user-data-dir", profileDir))
	} else {
		log.Println("Chromeプロファイルディレクトリが見つかりませんでした。デフォルト設定で起動します。")
	}

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	// chromedpのコンテキストを作成
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// ページに遷移し、imgタグのsrc属性をJavaScriptで取得
	var imgSrcs []string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(*pageURL),
		// ページのレンダリング待ち（必要に応じて調整）
		chromedp.Sleep(2*time.Second),
		// document.querySelectorAllで全imgタグのsrcを取得
		chromedp.Evaluate(`Array.from(document.querySelectorAll("img")).map(img => img.getAttribute("src"))`, &imgSrcs),
	); err != nil {
		log.Fatalf("chromedp実行エラー: %v", err)
	}

	// 各srcに対して絶対URLを生成し、コンソール出力・ダウンロードを実施
	for i, src := range imgSrcs {
		if src == "" {
			continue
		}

		// ベースURLとsrcを結合して絶対URLを生成
		imgURL, err := base.Parse(src)
		if err != nil {
			log.Printf("srcのパースに失敗しました [%s]: %v", src, err)
			continue
		}

		// URLをコンソールに出力
		fmt.Printf("Image %d: %s\n", i+1, imgURL.String())

		// ダウンロードするファイル名はURLの最後の名前（パスのベース名）を使用する
		fileName := filepath.Base(imgURL.Path)
		// ファイル名が取得できない場合は、連番＋拡張子でファイル名を生成する
		if fileName == "" || fileName == "/" || fileName == "." {
			fileName = fmt.Sprintf("image_%d%s", i+1, getFileExtension(imgURL.Path))
		}

		if err := downloadFile(imgURL.String(), *outDir, fileName); err != nil {
			log.Printf("画像のダウンロードに失敗しました [%s]: %v", imgURL.String(), err)
		}
	}
}

// downloadFileは指定URLからデータを取得し、outDir/fileNameとして保存します。
func downloadFile(urlStr, outDir, fileName string) error {
	resp, err := http.Get(urlStr)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTPステータスがOKではありません: %s", resp.Status)
	}

	filePath := filepath.Join(outDir, fileName)
	outFile, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	return err
}

// getFileExtensionはURLパスから拡張子を取得し、なければ".jpg"を返します。
func getFileExtension(path string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return ".jpg"
	}
	return ext
}

// getChromeProfileDirはOSごとのカレントユーザのChromeプロファイルディレクトリのパスを返します。
func getChromeProfileDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Printf("ユーザのホームディレクトリの取得に失敗: %v", err)
		return ""
	}

	switch runtime.GOOS {
	case "windows":
		// Windowsの場合: %LOCALAPPDATA%\Google\Chrome\User Data\Default
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			return ""
		}
		return filepath.Join(localAppData, "Google", "Chrome", "User Data", "Default")
	case "darwin":
		// macOSの場合: ~/Library/Application Support/Google/Chrome/Default
		return filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default")
	case "linux":
		// Linuxの場合: ~/.config/google-chrome/Default
		return filepath.Join(home, ".config", "google-chrome", "Default")
	default:
		return ""
	}
}
