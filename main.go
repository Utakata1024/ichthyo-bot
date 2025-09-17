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
    "sync"
    "syscall"
    "time"

    "github.com/bwmarrin/discordgo"
    "github.com/joho/godotenv"
)

// Expressサーバーに送信するペイロードの構造体
type ExpressPayload struct {
    Query string `json:"query"`
}

// Expressサーバーからの成功時の応答の構造体
// サーバーの "content" 配列と "text" フィールドを正しく扱えるように修正
type ExpressSuccessResponse struct {
    Content []struct {
        Type string `json:"type"`
        Text string `json:"text"`
    } `json:"content"`
}

// Expressサーバーからのエラー時の応答の構造体
// エラーメッセージ用の "error" フィールドのみを持つ
type ExpressResponse struct {
    Error string `json:"error"`
}

// チャンネルIDを安全に管理するためのグローバル変数とMutex
var (
    targetChannelID string
    channelMutex    sync.Mutex
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

    // プログラムの終了を通知するチャネルを作成
    stopChan := make(chan struct{})

    // 1時間ごとにメッセージを送信するGoroutineを起動
    go func() {
        // time.Hour を使用して1時間ごとにティックを発生させる
        ticker := time.NewTicker(10 * time.Second) // テスト用に10秒に設定。本番では1*time.Hourに変更してください。
        defer ticker.Stop()

        for {
            select {
            case <-ticker.C:
                channelMutex.Lock()
                channelID := targetChannelID
                channelMutex.Unlock()

                if channelID != "" {
                    sendRecommendedSong(dg, channelID, "おすすめソング")
                } else {
                    log.Println("警告: 送信先チャンネルが設定されていません。")
                }
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
        channelMutex.Lock()
        targetChannelID = m.ChannelID
        channelMutex.Unlock()

        s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("このチャンネルを定期メッセージの送信先に設定しました！(ID: %s)", m.ChannelID))
        return
    }

    if strings.HasPrefix(content, "!send ") {
        messageToSend := strings.TrimPrefix(content, "!send ")

        payload := ExpressPayload{
            Query: messageToSend,
        }
        jsonData, err := json.Marshal(payload)
        if err != nil {
            log.Printf("ペイロードのJSONエンコードに失敗しました: %v", err)
            s.ChannelMessageSend(m.ChannelID, "内部エラーが発生しました。")
            return
        }

        expressServerURL := "http://127.0.0.1:4000/api/tool/search-track"
        resp, err := http.Post(expressServerURL, "application/json", bytes.NewBuffer(jsonData))
        if err != nil {
            log.Printf("Expressサーバーへのリクエスト送信に失敗しました: %v", err)
            s.ChannelMessageSend(m.ChannelID, "サーバーとの通信に失敗しました。")
            return
        }
        defer resp.Body.Close()

        log.Printf("Expressサーバーからの応答ステータス: %v", resp.StatusCode)

        // HTTPステータスコードで成功と失敗を判定
        if resp.StatusCode == http.StatusOK {
            var expressResponse ExpressSuccessResponse
            if err := json.NewDecoder(resp.Body).Decode(&expressResponse); err != nil {
                log.Printf("Expressサーバーからの応答解析に失敗しました: %v", err)
                s.ChannelMessageSend(m.ChannelID, "サーバーからの応答解析に失敗しました。")
                return
            }
            if len(expressResponse.Content) > 0 && expressResponse.Content[0].Type == "text" {
                s.ChannelMessageSend(m.ChannelID, expressResponse.Content[0].Text)
            } else {
                s.ChannelMessageSend(m.ChannelID, "サーバーからの有効な応答が見つかりませんでした。")
            }
        } else {
            var expressError ExpressResponse
            if err := json.NewDecoder(resp.Body).Decode(&expressError); err != nil {
                log.Printf("Expressサーバーからのエラー応答解析に失敗しました: %v", err)
                s.ChannelMessageSend(m.ChannelID, "サーバーからのエラー応答解析に失敗しました。")
                return
            }
            s.ChannelMessageSend(m.ChannelID, "エラー: " + expressError.Error)
        }
        return
    }

    switch strings.ToLower(content) {
    case "!hello":
        s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Hello, %sさん！", m.Author.Username))
    case "!help":
        s.ChannelMessageSend(m.ChannelID, "以下に使い方を掲載\n!setchannel: このチャンネルを定期メッセージの送信先に設定します。\n!send <メッセージ>: Expressサーバーにメッセージを送信します。")
    }
}

// 1時間ごとにおすすめソングを送信する関数
func sendRecommendedSong(s *discordgo.Session, channelID, query string) {
    payload := ExpressPayload{
        Query: query,
    }
    jsonData, err := json.Marshal(payload)
    if err != nil {
        log.Printf("ペイロードのJSONエンコードに失敗しました: %v", err)
        return
    }

    expressServerURL := "http://127.0.0.1:4000/api/tool/search-track"
    resp, err := http.Post(expressServerURL, "application/json", bytes.NewBuffer(jsonData))
    if err != nil {
        log.Printf("Expressサーバーへのリクエスト送信に失敗しました: %v", err)
        s.ChannelMessageSend(channelID, "サーバーとの通信に失敗しました。")
        return
    }
    defer resp.Body.Close()

    log.Printf("Expressサーバーからの応答ステータス: %v", resp.StatusCode)

    if resp.StatusCode == http.StatusOK {
        var expressResponse ExpressSuccessResponse
        if err := json.NewDecoder(resp.Body).Decode(&expressResponse); err != nil {
            log.Printf("Expressサーバーからの応答解析に失敗しました: %v", err)
            s.ChannelMessageSend(channelID, "サーバーからの応答解析に失敗しました。")
            return
        }
        if len(expressResponse.Content) > 0 && expressResponse.Content[0].Type == "text" {
            s.ChannelMessageSend(channelID, "おすすめソング: " + expressResponse.Content[0].Text)
        } else {
            s.ChannelMessageSend(channelID, "サーバーからの有効な応答が見つかりませんでした。")
        }
    } else {
        var expressError ExpressResponse
        if err := json.NewDecoder(resp.Body).Decode(&expressError); err != nil {
            log.Printf("Expressサーバーからのエラー応答解析に失敗しました: %v", err)
            s.ChannelMessageSend(channelID, "サーバーからのエラー応答解析に失敗しました。")
            return
        }
        s.ChannelMessageSend(channelID, "エラー: " + expressError.Error)
    }
}
