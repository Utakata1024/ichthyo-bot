package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

// Expressサーバーに送信する分類用のペイロード
type ClassifyPayload struct {
	Query string `json:"query"`
}

// Expressサーバーからの分類成功時の応答
type ClassifySuccessResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

// Expressサーバーに送信する検索用のペイロード
type SearchPayload struct {
	Type    string `json:"type"`
	Keyword string `json:"keyword"`
}

// Expressサーバーからの検索成功時の応答
type SearchSuccessResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// Expressサーバーからのエラー時の応答
type ExpressErrorResponse struct {
	Error string `json:"error"`
}

// チャンネルID、キーワード、間隔を安全に管理するためのグローバル変数とMutex
var (
	targetChannelID        string
	recommendKeyword       string = "おすすめソング" // 初期値を設定
	recommendInterval      time.Duration = 1 * time.Minute // 初期値を設定
	channelAndKeywordMutex sync.Mutex
)

// タイマーの再起動を通知するためのチャネル
var restartTickerChan = make(chan struct{}, 1)

func main() {
	// .envファイルを読み込む
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	// 環境変数からBotトークンを読み込む
	token := os.Getenv("DISCORD_BOT_TOKEN")
	if token == "" {
		fmt.Println("環境変数 DISCORD_BOT_TOKEN が設定されていません")
		return
	}

	// Discordセッションを作成
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		fmt.Println("Discordセッションの作成に失敗しました:", err)
		return
	}

	// Intentを設定
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	// メッセージを受信したときのイベントハンドラを登録
	dg.AddHandler(messageCreate)

	// WebSocket接続を開く
	err = dg.Open()
	if err != nil {
		fmt.Println("WebSocket接続に失敗しました:", err)
		return
	}

	// プログラムの終了を通知するチャネルを作成
	stopChan := make(chan struct{})

	// 1時間ごとにメッセージを送信するGoroutineを起動
	go func() {
		// 初期設定の間隔でtickerを作成
		channelAndKeywordMutex.Lock()
		ticker := time.NewTicker(recommendInterval)
		channelAndKeywordMutex.Unlock()
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				channelAndKeywordMutex.Lock()
				channelID := targetChannelID
				keyword := recommendKeyword
				channelAndKeywordMutex.Unlock()

				if channelID != "" {
					sendRecommendedSong(dg, channelID, keyword)
				} else {
					log.Println("警告: 送信先チャンネルが設定されていません。")
				}
			case <-restartTickerChan:
				// 新しい間隔が設定されたらタイマーを再起動
				ticker.Stop()
				channelAndKeywordMutex.Lock()
				ticker = time.NewTicker(recommendInterval)
				channelAndKeywordMutex.Unlock()
				log.Println("定期実行のタイマーを再起動しました。")
			case <-stopChan:
				// 終了チャネルが閉じられたらループを抜ける
				return
			}
		}
	}()

	// プログラムが終了しないように待機
	fmt.Println("Botが起動しました。CTRL-Cで終了します。")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// 終了処理
	fmt.Println("Botを終了します...")
	close(stopChan) // 定期実行のGoroutineに終了を通知
	dg.Close()
}

// メッセージ作成イベントが発生したときに呼ばれる関数
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	content := strings.TrimSpace(m.Content)

	if strings.ToLower(content) == "!setchannel" {
		channelAndKeywordMutex.Lock()
		targetChannelID = m.ChannelID
		channelAndKeywordMutex.Unlock()

		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("このチャンネルを定期メッセージの送信先に設定しました！(ID: %s)", m.ChannelID))
		return
	}

	// 新しいコマンド: レコメンドキーワードの設定
	if strings.HasPrefix(content, "!setkeyword ") {
		newKeyword := strings.TrimPrefix(content, "!setkeyword ")
		if newKeyword == "" {
			s.ChannelMessageSend(m.ChannelID, "キーワードが指定されていません。例: `!setkeyword J-Pop`")
			return
		}
		channelAndKeywordMutex.Lock()
		recommendKeyword = newKeyword
		channelAndKeywordMutex.Unlock()
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("定期メッセージの検索キーワードを「%s」に設定しました。", newKeyword))
		return
	}

	// 新しいコマンド: レコメンド頻度の設定
	if strings.HasPrefix(content, "!setinterval ") {
		parts := strings.Split(strings.TrimPrefix(content, "!setinterval "), " ")
		if len(parts) != 2 {
			s.ChannelMessageSend(m.ChannelID, "フォーマットが正しくありません。例: `!setinterval 10 minutes`")
			return
		}

		value, err := strconv.Atoi(parts[0])
		if err != nil || value <= 0 {
			s.ChannelMessageSend(m.ChannelID, "無効な数値です。1以上の数値を指定してください。")
			return
		}

		unit := strings.ToLower(parts[1])
		var duration time.Duration
		switch unit {
		case "minutes", "minute":
			duration = time.Duration(value) * time.Minute
		case "hours", "hour":
			duration = time.Duration(value) * time.Hour
		default:
			s.ChannelMessageSend(m.ChannelID, "無効な単位です。`minutes`または`hours`を指定してください。")
			return
		}

		channelAndKeywordMutex.Lock()
		recommendInterval = duration
		channelAndKeywordMutex.Unlock()

		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("定期メッセージの頻度を%d %sに設定しました。", value, unit))
		// タイマーを再起動するために信号を送信
		restartTickerChan <- struct{}{}
		return
	}

	if strings.HasPrefix(content, "!send ") {
		messageToSend := strings.TrimPrefix(content, "!send ")
		
		// ステップ1: 分類APIを呼び出してキーワードを取得
		keyword, err := classifyQuery(messageToSend)
		if err != nil {
			log.Printf("分類API呼び出しに失敗: %v", err)
			s.ChannelMessageSend(m.ChannelID, "検索キーワードの取得中にエラーが発生しました。")
			return
		}

		// ステップ2: 検索APIを呼び出して曲情報を取得
		responseText, err := searchSpotify(keyword)
		if err != nil {
			log.Printf("検索API呼び出しに失敗: %v", err)
			s.ChannelMessageSend(m.ChannelID, "Spotifyの検索中にエラーが発生しました。")
			return
		}

		// Discordに結果を送信
		s.ChannelMessageSend(m.ChannelID, responseText)
		return
	}

	switch strings.ToLower(content) {
	case "!hello":
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Hello, %sさん！", m.Author.Username))
	case "!help":
		s.ChannelMessageSend(m.ChannelID, "以下に使い方を掲載\n!setchannel: このチャンネルを定期メッセージの送信先に設定します。\n!setkeyword <キーワード>: 定期メッセージの検索キーワードを設定します。\n!setinterval <数値> <単位>: 定期メッセージの頻度を設定します (例: `!setinterval 10 minutes`)\n!send <メッセージ>: Expressサーバーにメッセージを送信します。")
	}
}

// 分類APIを呼び出してキーワードを取得する関数
func classifyQuery(query string) (string, error) {
	payload := ClassifyPayload{Query: query}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("ペイロードのJSONエンコードに失敗しました: %w", err)
	}

	expressServerURL := "http://127.0.0.1:4000/api/tool/classify-spotify-query"
	resp, err := http.Post(expressServerURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("Expressサーバーへのリクエスト送信に失敗しました: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp ExpressErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Error != "" {
			return "", fmt.Errorf("サーバーからのエラー応答: %s", errResp.Error)
		}
		return "", fmt.Errorf("サーバーから予期しないステータスコードが返されました: %d", resp.StatusCode)
	}

	var successResp ClassifySuccessResponse
	if err := json.NewDecoder(resp.Body).Decode(&successResp); err != nil {
		return "", fmt.Errorf("サーバーからの応答解析に失敗しました: %w", err)
	}

	if len(successResp.Content) > 0 && successResp.Content[0].Text != "" {
		return successResp.Content[0].Text, nil
	}

	return "", fmt.Errorf("分類結果が見つかりませんでした")
}

// 検索APIを呼び出して曲情報を取得する関数 (変更なし)
func searchSpotify(keyword string) (string, error) {
	payload := SearchPayload{
		Type:    "track", // ここでは常にトラックを検索
		Keyword: keyword,
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("ペイロードのJSONエンコードに失敗しました: %w", err)
	}

	expressServerURL := "http://127.0.0.1:4000/api/tool/search-spotify"
	resp, err := http.Post(expressServerURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("Expressサーバーへのリクエスト送信に失敗しました: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp ExpressErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Error != "" {
			return "", fmt.Errorf("サーバーからのエラー応答: %s", errResp.Error)
		}
		return "", fmt.Errorf("サーバーから予期しないステータスコードが返されました: %d", resp.StatusCode)
	}

	var successResp SearchSuccessResponse
	if err := json.NewDecoder(resp.Body).Decode(&successResp); err != nil {
		return "", fmt.Errorf("サーバーからの応答解析に失敗しました: %w", err)
	}

	if len(successResp.Content) > 0 && successResp.Content[0].Text != "" {
		return successResp.Content[0].Text, nil
	}

	return "検索結果が見つかりませんでした", nil
}

// 定期実行でおすすめソングを送信する関数
func sendRecommendedSong(s *discordgo.Session, channelID, query string) {
	// ステップ1: 分類APIを呼び出してキーワードを取得
	keyword, err := classifyQuery(query)
	if err != nil {
		log.Printf("定期実行: 分類API呼び出しに失敗: %v", err)
		s.ChannelMessageSend(channelID, "定期メッセージ: 検索キーワードの取得中にエラーが発生しました。")
		return
	}

	// ステップ2: 検索APIを呼び出して曲情報を取得
	responseText, err := searchSpotify(keyword)
	if err != nil {
		log.Printf("定期実行: 検索API呼び出しに失敗: %v", err)
		s.ChannelMessageSend(channelID, "定期メッセージ: Spotifyの検索中にエラーが発生しました。")
		return
	}

	// Discordに結果を送信
	s.ChannelMessageSend(channelID, "おすすめソング: " + responseText)
}
