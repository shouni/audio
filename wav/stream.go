package wav

import (
	"encoding/binary"
	"fmt"
	"io"
)

// maxCarriedHeaderSize は、先頭ファイルから引き継ぐヘッダー（data チャンク直前まで）の
// 上限です。ストリーミング結合はメモリ使用量を入力サイズから切り離すのが目的なので、
// 巨大なメタデータチャンクを丸ごと読み込んでその前提を壊さないよう歯止めを置きます。
const maxCarriedHeaderSize = 1 << 20 // 1MiB

// streamParts は1つの WAV について、走査で判明した位置とフォーマットです。
type streamParts struct {
	format Format
	// headerSize は data チャンクヘッダーが始まる位置、つまり引き継ぐヘッダーの長さです。
	headerSize int64
	// dataOffset は data チャンクのペイロードが始まる位置です。
	dataOffset int64
	// dataSize は data チャンクのペイロードのバイト数です。
	dataSize int64
}

// CombineTo は複数の WAV を結合し、結果を w へ書き出します。
//
// 入力を丸ごとメモリへ載せる CombineWavData と違い、音声データを入力から w へ直接
// 受け渡すため、必要なメモリは音声の長さにも本数にも比例しません。ファイル同士の結合や
// 長尺の書き出しにはこちらを使ってください。
//
// sources は 2 回走査します。1 回目でフォーマットと data チャンクの位置を調べ、
// 2 回目で音声を転送します。出力の RIFF/data チャンクサイズは先頭に書く必要があり、
// 全体の長さが分かるまで確定しないためです。したがって入力には io.ReadSeeker が要ります。
//
// 検証内容とエラーは CombineWavData と同じです。
func CombineTo(w io.Writer, sources []io.ReadSeeker, opts ...CombineOption) error {
	if len(sources) == 0 {
		return &ErrNoAudioData{}
	}
	cfg := newCombineConfig(opts)

	parts := make([]streamParts, len(sources))
	var totalAudioSize uint64
	for i, source := range sources {
		part, err := scanStream(source, i)
		if err != nil {
			return err
		}
		// 出力ヘッダーは先頭ファイルのものを使うため、フォーマットが違うと
		// 2本目以降が別フォーマットとして再生されてしまう。
		if i > 0 {
			if err := verifySameFormat(parts[0].format, part.format, i); err != nil {
				return err
			}
		}
		parts[i] = part
		totalAudioSize += uint64(part.dataSize)
	}

	silence := cfg.silence(parts[0].format)
	totalAudioSize += uint64(len(silence)) * uint64(len(sources)-1)

	formatHeader, err := readCarriedHeader(sources[0], parts[0].headerSize)
	if err != nil {
		return err
	}
	header, err := buildCombinedHeader(formatHeader, totalAudioSize)
	if err != nil {
		return err
	}
	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("WAVヘッダーの書き出しに失敗しました: %w", err)
	}

	for i, source := range sources {
		if i > 0 && len(silence) > 0 {
			if _, err := w.Write(silence); err != nil {
				return fmt.Errorf("無音の書き出しに失敗しました: %w", err)
			}
		}
		if _, err := source.Seek(parts[i].dataOffset, io.SeekStart); err != nil {
			return streamIOError(i, err)
		}
		if _, err := io.CopyN(w, source, parts[i].dataSize); err != nil {
			return fmt.Errorf("WAVファイル #%d の音声データの転送に失敗しました: %w", i, err)
		}
	}
	return nil
}

// scanStream は WAV を走査し、フォーマットと data チャンクの位置を調べます。
// 音声データそのものは読み込まず、チャンクヘッダーだけを辿ります。
func scanStream(source io.ReadSeeker, index int) (streamParts, error) {
	size, err := source.Seek(0, io.SeekEnd)
	if err != nil {
		return streamParts{}, streamIOError(index, err)
	}

	var riffHeader [wavRiffHeaderSize]byte
	if err := readAt(source, 0, riffHeader[:], index); err != nil {
		if size < wavRiffHeaderSize {
			return streamParts{}, &ErrInvalidWAVHeader{
				Index:   index,
				Details: fmt.Sprintf("WAVファイルサイズが短すぎます (RIFFヘッダー不足: %dバイト)", size),
			}
		}
		return streamParts{}, err
	}
	if err := validateRiffHeader(riffHeader[:], index); err != nil {
		return streamParts{}, err
	}

	var (
		format        Format
		fmtChunkFound bool
	)
	for offset := int64(wavRiffHeaderSize); offset+chunkHeaderSize <= size; {
		var chunkHeader [chunkHeaderSize]byte
		if err := readAt(source, offset, chunkHeader[:], index); err != nil {
			return streamParts{}, err
		}
		id := string(chunkHeader[:chunkIDSize])
		chunkSize := binary.LittleEndian.Uint32(chunkHeader[chunkIDSize:])

		switch id {
		case "fmt ":
			payloadStart := offset + chunkHeaderSize
			payload := make([]byte, min(int64(chunkSize), extensibleFormatChunkSize, size-payloadStart))
			if err := readAt(source, payloadStart, payload, index); err != nil {
				return streamParts{}, err
			}
			if format, err = parseFormatPayload(payload, chunkSize, index); err != nil {
				return streamParts{}, err
			}
			fmtChunkFound = true
		case "data":
			if !fmtChunkFound {
				return streamParts{}, missingChunkError(index, "'fmt '")
			}
			dataOffset := offset + chunkHeaderSize
			if int64(chunkSize) > size-dataOffset {
				return streamParts{}, &ErrInvalidWAVHeader{
					Index:   index,
					Details: "dataチャンクのデータ長が実際のファイルサイズを超過しています",
				}
			}
			return streamParts{
				format:     format,
				headerSize: offset,
				dataOffset: dataOffset,
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
		return streamParts{}, missingChunkError(index, "'data'")
	}
	return streamParts{}, missingChunkError(index, "'fmt '", "'data'")
}

// readCarriedHeader は先頭ファイルから、そのまま出力へ引き継ぐヘッダーを読み込みます。
func readCarriedHeader(source io.ReadSeeker, headerSize int64) ([]byte, error) {
	if headerSize > maxCarriedHeaderSize {
		return nil, &ErrInvalidWAVHeader{
			Index:   0,
			Details: fmt.Sprintf("dataチャンクより前のヘッダーが大きすぎます (%dバイト、上限%dバイト)。CombineWavData を使ってください", headerSize, maxCarriedHeaderSize),
		}
	}
	header := make([]byte, headerSize)
	if err := readAt(source, 0, header, 0); err != nil {
		return nil, err
	}
	return header, nil
}

// readAt は offset から buf を埋めるだけ読み込みます。
func readAt(source io.ReadSeeker, offset int64, buf []byte, index int) error {
	if _, err := source.Seek(offset, io.SeekStart); err != nil {
		return streamIOError(index, err)
	}
	if _, err := io.ReadFull(source, buf); err != nil {
		return streamIOError(index, err)
	}
	return nil
}

func streamIOError(index int, err error) error {
	return fmt.Errorf("WAVファイル #%d の読み込みに失敗しました: %w", index, err)
}
