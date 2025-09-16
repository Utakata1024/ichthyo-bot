package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

// Webhookに送信するJSONデータの構造体
type WebhookMessage struct {
	Content string `json:"content"`
}

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

	// プログラムが終了しないように待機
	fmt.Println("Botが起動しました。CTRL-Cで終了します。")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// 終了処理
	dg.Close()
}

// メッセージ作成イベントが発生したときに呼ばれる関数
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Bot自身のメッセージには反応しないようにする
	if m.Author.ID == s.State.User.ID {
		return
	}

	// メッセージの内容を取得
	content := strings.TrimSpace(m.Content)

	// '!send ' コマンドの処理
	if strings.HasPrefix(content, "!send ") {
		// コマンドの後に続くメッセージを取得
		messageToSend := strings.TrimPrefix(content, "!send ")

		// MCPサーバーにメッセージを送信し、Spotify APIの処理を行う関数を呼び出す
        // ここでは、仮の関数として、Discordにメッセージを返信
        s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("MCPサーバーに「%s」を送信します。", messageToSend))
        
        // 実際の連携処理は、この行を置き換えて実装します。
        // 例: sendToMCPAndSpotify(messageToSend)

		// 他のコマンドはスキップ
		return
	}

	// その他の既存コマンドも引き続き動作
	switch strings.ToLower(content) {
	case "!hello":
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Hello, %sさん！", m.Author.Username))
	case "!about":
		s.ChannelMessageSend(m.ChannelID, "私はGoで作られたDiscordボットです。")
	}
}

// Webhookにメッセージを送信する関数
func sendWebhookMessage(messageContent string) {
	// 環境変数からWebhook URLを読み込む
	webhookURL := os.Getenv("DISCORD_WEBHOOK_URL")
	if webhookURL == "" {
		fmt.Println("Webhook URLが設定されていません。")
		return
	}

	// Webhookに送信するメッセージを作成
	message := WebhookMessage{
		Content: messageContent,
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		fmt.Println("JSONエンコードに失敗しました:", err)
		return
	}

	// HTTPリクエストの作成と送信
	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("リクエスト作成に失敗しました:", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("リクエスト送信に失敗しました:", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		fmt.Printf("メッセージの送信に失敗しました。ステータスコード: %d\n", resp.StatusCode)
	}
}