package main

import (
    "fmt"
    "log"
    "os"
    "os/signal"
    "strings"
    "syscall"

    "github.com/bwmarrin/discordgo"
    "github.com/joho/godotenv"
)

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

// // Expressサーバーに送信するペイロードの構造体
// type ExpressPayload struct {
// 	User    string `json:"user"`
// 	Message string `json:"message"`
// }

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
        
        // MCPサーバーにメッセージを送信し、Spotify APIの処理を行う
        // この部分に実際のロジックを実装します。
        s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("MCPサーバーに「%s」を送信します。", messageToSend))

		// Expressサーバーに送信するペイロードを作成
        payload := ExpressPayload{
            User:    m.Author.Username,
            Message: messageToSend,
        }
        jsonData, err := json.Marshal(payload)
        if err != nil {
            log.Printf("ペイロードのJSONエンコードに失敗しました: %v", err)
            s.ChannelMessageSend(m.ChannelID, "サーバーとの通信でエラーが発生しました。")
            return
        }

        // // Expressサーバーのエンドポイント
        // expressServerURL := "http://localhost:3000/discord-message"
        // resp, err := http.Post(expressServerURL, "application/json", bytes.NewBuffer(jsonData))
        // if err != nil {
        //     log.Printf("Expressサーバーへのリクエスト送信に失敗しました: %v", err)
        //     s.ChannelMessageSend(m.ChannelID, "サーバーとの通信に失敗しました。")
        //     return
        // }
        // defer resp.Body.Close()

        // // Expressサーバーからの応答を解析
        // var expressResponse map[string]string
        // if err := json.NewDecoder(resp.Body).Decode(&expressResponse); err != nil {
        //     log.Printf("Expressサーバーからの応答解析に失敗しました: %v", err)
        //     s.ChannelMessageSend(m.ChannelID, "サーバーからの応答解析に失敗しました。")
        //     return
        // }

        // // Expressサーバーからの応答をDiscordに送信
        // if expressResponse["status"] == "success" {
        //     s.ChannelMessageSend(m.ChannelID, expressResponse["response"])
        // } else {
        //     s.ChannelMessageSend(m.ChannelID, "エラー: " + expressResponse["message"])
        // }

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