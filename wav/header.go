package wav

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

// RIFF 構造の解析に使用するサイズ定数です。
const (
	// chunkIDSize は、チャンクID ("RIFF", "fmt ", "data" 等) のサイズ（バイト）です。
	chunkIDSize = 4
	// chunkSizeSize は、チャンクサイズフィールドのサイズ（バイト）です。
	chunkSizeSize = 4
	// waveIDSize は "WAVE" 識別子のサイズ（バイト）です。
	waveIDSize = 4
)

// WAV ファイルのヘッダー計算で使用される複合サイズ定数です。
const (
	// standardHeaderSize は、拡張チャンクを持たない WAV のヘッダーサイズ（44バイト）です。
	standardHeaderSize = 44
	// chunkHeaderSize は、チャンクヘッダー (ID + サイズ) の合計サイズ（8バイト）です。
	chunkHeaderSize = chunkIDSize + chunkSizeSize
	// minFormatChunkSize は PCM の "fmt " チャンクのペイロードサイズ（16バイト）です。
	minFormatChunkSize = 16
	// extensibleFormatChunkSize は WAVE_FORMAT_EXTENSIBLE の "fmt " チャンクの
	// ペイロードサイズ（40バイト）です。
	extensibleFormatChunkSize = 40
	// wavRiffHeaderSize は RIFF ヘッダーの合計サイズ（12バイト）です。
	wavRiffHeaderSize = chunkIDSize + chunkSizeSize + waveIDSize
)

// riffChunkSizeOffset は、結合時に RIFF チャンクサイズを書き換えるオフセット位置（4バイト目）です。
const riffChunkSizeOffset = chunkIDSize

// extensibleFormatTag は WAVE_FORMAT_EXTENSIBLE を表す AudioFormat の値です。
// この値のときだけ、実際の符号化方式は fmt チャンク末尾の SubFormat GUID 側にあります。
const extensibleFormatTag = 0xFFFE

// wavParts は1つの WAV から取り出した、結合に必要な部品です。
type wavParts struct {
	// formatHeader は data チャンク直前までの、そのまま出力へ引き継ぐヘッダーです。
	formatHeader []byte
	format       Format
	audioData    []byte
}

// wavLayout は1つの WAV について、走査で判明した位置とフォーマットです。
type wavLayout struct {
	format Format
	// headerSize は data チャンクヘッダーが始まる位置、つまり引き継ぐヘッダーの長さです。
	headerSize int64
	// dataOffset は data チャンクのペイロードが始まる位置です。
	dataOffset int64
	// dataSize は data チャンクのペイロードのバイト数です。
	dataSize int64
}

// chunkSource は走査対象の WAV で、メモリ上のバイト列か io.ReadSeeker のどちらかを持ちます。
//
// 両方をこの形に揃えることで、チャンク走査と検証を scanWAV の 1 本に置き、
// CombineWavData と CombineTo が受け付ける入力と返すエラーが食い違わないようにします。
//
// interface ではなく 1 つの構造体にしているのは割り当てのためです。interface のメソッド
// 呼び出しは引数がヒープへ逃げるので、走査のたびに読み出しバッファが確保されてしまい、
// Inspect が割り当てなしで済むという性質が崩れます。
type chunkSource struct {
	// data はメモリ上の入力です。stream が nil のときに使います。
	data   []byte
	stream io.ReadSeeker
	// index はエラーメッセージに含めるファイル位置です。
	index int
	// total は WAV 全体のバイト数です。
	total int64
	// buf は stream からの固定長読み出しに使います。fmt チャンクのペイロードが最大です。
	// io.ReadFull へ渡すとヒープへ逃げるため、配列を構造体に埋め込まずストリームの
	// ときだけ確保します。こうするとバイト列の走査は割り当てなしで済みます。
	buf []byte
}

// bytesSource はメモリ上のバイト列に対する chunkSource を返します。
func bytesSource(data []byte, index int) chunkSource {
	return chunkSource{data: data, index: index, total: int64(len(data))}
}

// streamSource は io.ReadSeeker に対する chunkSource を返します。
// 末尾まで Seek して全体のバイト数を確かめます。
func streamSource(r io.ReadSeeker, index int) (chunkSource, error) {
	total, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return chunkSource{}, streamIOError(index, err)
	}
	return chunkSource{stream: r, index: index, total: total, buf: make([]byte, extensibleFormatChunkSize)}, nil
}

// readAt は offset から n バイトを返します。バイト列ならその部分スライスを、ストリームなら
// 内部バッファへ読み込んだものを返すので、返り値は次の readAt まで有効です。
// 呼び出し元は offset+n が total を超えないことと、n が buf に収まることを保証します。
func (s *chunkSource) readAt(offset, n int64) ([]byte, error) {
	if s.stream == nil {
		if offset < 0 || offset+n > int64(len(s.data)) {
			return nil, io.ErrUnexpectedEOF
		}
		return s.data[offset : offset+n], nil
	}
	buf := s.buf[:n]
	if err := readFull(s.stream, offset, buf, s.index); err != nil {
		return nil, err
	}
	return buf, nil
}

// readFull は r の offset から buf を埋めるだけ読み込みます。
func readFull(r io.ReadSeeker, offset int64, buf []byte, index int) error {
	if _, err := r.Seek(offset, io.SeekStart); err != nil {
		return streamIOError(index, err)
	}
	if _, err := io.ReadFull(r, buf); err != nil {
		return streamIOError(index, err)
	}
	return nil
}

func streamIOError(index int, err error) error {
	return fmt.Errorf("WAVファイル #%d の読み込みに失敗しました: %w", index, err)
}

// extractAudioData は WAV ファイルからフォーマット情報と音声データ部分を抽出します。
// fmt および data チャンクを動的に探索し、data チャンクの直前までを formatHeader とします。
// index はエラーメッセージに含めるファイル位置で、一覧に属さない検証では -1 を渡します。
func extractAudioData(wavBytes []byte, index int) (wavParts, error) {
	src := bytesSource(wavBytes, index)
	layout, err := scanWAV(&src)
	if err != nil {
		return wavParts{}, err
	}
	dataStart := int(layout.dataOffset)
	return wavParts{
		formatHeader: wavBytes[:int(layout.headerSize)],
		format:       layout.format,
		audioData:    wavBytes[dataStart : dataStart+int(layout.dataSize)],
	}, nil
}

// scanWAV は WAV を走査し、フォーマットと data チャンクの位置を調べます。
//
// 音声データそのものは読み込まず、RIFF ヘッダー・各チャンクのヘッダー・fmt チャンクの
// ペイロードだけを読みます。読む量はどれも固定長なので、走査に必要なメモリは
// 入力の大きさに依存しません。
func scanWAV(src *chunkSource) (wavLayout, error) {
	index, size := src.index, src.total
	if size < wavRiffHeaderSize {
		return wavLayout{}, &ErrInvalidWAVHeader{
			Index:   index,
			Details: fmt.Sprintf("WAVファイルサイズが短すぎます (RIFFヘッダー不足: %dバイト)", size),
		}
	}
	riffHeader, err := src.readAt(0, wavRiffHeaderSize)
	if err != nil {
		return wavLayout{}, err
	}
	if err := validateRiffHeader(riffHeader, index); err != nil {
		return wavLayout{}, err
	}

	var (
		format        Format
		fmtChunkFound bool
	)
	for offset := int64(wavRiffHeaderSize); offset+chunkHeaderSize <= size; {
		chunkHeader, err := src.readAt(offset, chunkHeaderSize)
		if err != nil {
			return wavLayout{}, err
		}
		chunkSize := binary.LittleEndian.Uint32(chunkHeader[chunkIDSize:])
		payloadStart := offset + chunkHeaderSize

		switch string(chunkHeader[:chunkIDSize]) {
		case "fmt ":
			// 拡張部まで含めても 40 バイト。宣言サイズと実際の残量のうち短い方まで読む。
			payload, err := src.readAt(payloadStart, min(int64(chunkSize), extensibleFormatChunkSize, size-payloadStart))
			if err != nil {
				return wavLayout{}, err
			}
			parsed, err := parseFormatPayload(payload, chunkSize, index)
			if err != nil {
				return wavLayout{}, err
			}
			format = parsed
			fmtChunkFound = true
		case "data":
			if !fmtChunkFound {
				return wavLayout{}, missingChunkError(index, "'fmt '")
			}
			if int64(chunkSize) > size-payloadStart {
				return wavLayout{}, &ErrInvalidWAVHeader{
					Index:   index,
					Details: "dataチャンクのデータ長が実際のファイルサイズを超過しています",
				}
			}
			return wavLayout{
				format:     format,
				headerSize: offset,
				dataOffset: payloadStart,
				dataSize:   int64(chunkSize),
			}, nil
		}

		nextOffset := nextChunkOffset(uint64(offset), chunkSize)
		if nextOffset > uint64(size) {
			break
		}
		offset = int64(nextOffset)
	}

	if fmtChunkFound {
		return wavLayout{}, missingChunkError(index, "'data'")
	}
	return wavLayout{}, missingChunkError(index, "'fmt '", "'data'")
}

// validateRiffHeader は WAV データの RIFF/WAVE 識別子を検証します。header は先頭 12 バイトです。
func validateRiffHeader(header []byte, index int) error {
	if string(header[:chunkIDSize]) != "RIFF" || string(header[chunkHeaderSize:wavRiffHeaderSize]) != "WAVE" {
		return &ErrInvalidWAVHeader{
			Index:   index,
			Details: "RIFF/WAVE識別子が不正です",
		}
	}
	return nil
}

// parseFormatPayload は fmt チャンクのペイロードからフォーマットを読み出します。
// declaredSize はチャンクヘッダーが宣言しているサイズで、payload はその先頭部分
// （最大 extensibleFormatChunkSize バイト）です。
//
// WAVE_FORMAT_EXTENSIBLE (AudioFormat = 0xFFFE) では実際の符号化方式もチャンネル配置も
// 先頭 16 バイトには入っていないため、拡張部（チャンネルマスクとサブフォーマット GUID）
// まで読み出します。ここを読まないと、リニア PCM と IEEE float が「どちらも 0xFFFE」
// として一致扱いになります。
func parseFormatPayload(payload []byte, declaredSize uint32, index int) (Format, error) {
	// PCM の fmt チャンクは 16 バイト。拡張形式はより長いが、先頭 16 バイトの並びは共通。
	if uint64(declaredSize) < minFormatChunkSize || len(payload) < minFormatChunkSize {
		return Format{}, &ErrInvalidWAVHeader{
			Index:   index,
			Details: fmt.Sprintf("fmtチャンクが短すぎます (%dバイト、最低%dバイト必要)", declaredSize, minFormatChunkSize),
		}
	}

	format := Format{
		AudioFormat:   binary.LittleEndian.Uint16(payload[0:]),
		NumChannels:   binary.LittleEndian.Uint16(payload[2:]),
		SampleRate:    binary.LittleEndian.Uint32(payload[4:]),
		BitsPerSample: binary.LittleEndian.Uint16(payload[14:]),
	}
	if format.AudioFormat != extensibleFormatTag {
		return format, nil
	}

	if uint64(declaredSize) < extensibleFormatChunkSize || len(payload) < extensibleFormatChunkSize {
		return Format{}, &ErrInvalidWAVHeader{
			Index:   index,
			Details: fmt.Sprintf("拡張形式(WAVE_FORMAT_EXTENSIBLE)のfmtチャンクが短すぎます (%dバイト、最低%dバイト必要)", declaredSize, extensibleFormatChunkSize),
		}
	}
	format.ChannelMask = binary.LittleEndian.Uint32(payload[20:])
	copy(format.SubFormat[:], payload[24:extensibleFormatChunkSize])
	return format, nil
}

// nextChunkOffset は WAV チャンクのパディングを考慮して次のチャンク位置を返します。
func nextChunkOffset(offset uint64, chunkSize uint32) uint64 {
	nextOffset := offset + uint64(chunkHeaderSize) + uint64(chunkSize)
	if chunkSize%2 != 0 {
		nextOffset++
	}
	return nextOffset
}

// missingChunkError は不足している必須チャンクを示すエラーを作成します。
func missingChunkError(index int, missing ...string) error {
	return &ErrInvalidWAVHeader{
		Index:   index,
		Details: fmt.Sprintf("WAVファイル内に必要なチャンク (%s) が見つかりませんでした", strings.Join(missing, "と")),
	}
}
