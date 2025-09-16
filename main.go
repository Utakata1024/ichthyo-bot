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

// Expressサーバーに送信するペイロードの構造体
type ExpressPayload struct {
	Query string `json:"query"`
}

// Expressサーバーからの応答の構造体
type ExpressResponse struct {
	Result string `json:"result"`
	Error  string `json:"error"`
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

		// Expressサーバーに送信するペイロードを作成
		payload := ExpressPayload{
			Query: messageToSend,
		}
		jsonData, err := json.Marshal(payload)
		if err != nil {
			log.Printf("ペイロードのJSONエンコードに失敗しました: %v", err)
			s.ChannelMessageSend(m.ChannelID, "内部エラーが発生しました。")
			return
		}

		// Expressサーバーのエンドポイント
		expressServerURL := "http://127.0.0.1:4000/api/tool/search-track"
		resp, err := http.Post(expressServerURL, "application/json", bytes.NewBuffer(jsonData))
		log.Printf("Expressサーバーへのリクエスト送信: %v", resp)
		log.Printf("respボディ: %v", resp.Body)
		if err != nil {
			log.Printf("Expressサーバーへのリクエスト送信に失敗しました: %v", err)
			s.ChannelMessageSend(m.ChannelID, "サーバーとの通信に失敗しました。")
			return
		}
		defer resp.Body.Close()

		// Expressサーバーからの応答を解析
		var expressResponse ExpressResponse
		if err := json.NewDecoder(resp.Body).Decode(&expressResponse); err != nil {
			log.Printf("Expressサーバーからの応答解析に失敗しました: %v", err)
			s.ChannelMessageSend(m.ChannelID, "サーバーからの応答解析に失敗しました。")
			return
		}

		// Expressサーバーからの応答をDiscordに送信
		if expressResponse.Error != "" {
			s.ChannelMessageSend(m.ChannelID, "エラー: " + expressResponse.Error)
		} else {
			s.ChannelMessageSend(m.ChannelID, expressResponse.Result)
		}

		return
	}

	// その他の既存コマンドも引き続き動作
	switch strings.ToLower(content) {
	case "!hello":
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Hello, %sさん！", m.Author.Username))
	case "!help":
		s.ChannelMessageSend(m.ChannelID, "以下に使い方を掲載")
	}
}