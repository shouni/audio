package wav

import "fmt"

// ErrNoAudioData は、結合対象の音声データがない場合に発生します。
type ErrNoAudioData struct{}

func (e *ErrNoAudioData) Error() string {
	return "結合対象の音声データがありません"
}

// ErrInvalidWAVHeader は、WAV ヘッダーの検証に失敗した場合に発生します。
type ErrInvalidWAVHeader struct {
	// Index はエラーが発生した WAV ファイルのインデックスです。
	Index int
	// Details はエラーの詳細情報です。
	Details string
}

func (e *ErrInvalidWAVHeader) Error() string {
	if e.Index >= 0 {
		return fmt.Sprintf("WAVファイル #%d のヘッダーが無効です: %s", e.Index, e.Details)
	}
	return fmt.Sprintf("WAVヘッダーが無効です: %s", e.Details)
}
