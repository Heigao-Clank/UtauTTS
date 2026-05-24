package audio

import (
    "bufio"
    "encoding/binary"
    "errors"
    "io"
    "os"
)

type PCM struct {
    SampleRate int
    Channels   int
    Data       []int16
}

func ReadWav(path string) (*PCM, error) {
    file, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer file.Close()

    reader := bufio.NewReader(file)
    if err := readString(reader, 4, "RIFF"); err != nil {
        return nil, err
    }
    if _, err := readUint32(reader); err != nil {
        return nil, err
    }
    if err := readString(reader, 4, "WAVE"); err != nil {
        return nil, err
    }

    var (
        fmtFound  bool
        dataFound bool
        sampleRate uint32
        channels   uint16
        bitsPerSample uint16
        pcmData []int16
    )

    for {
        chunkID, err := readChunkID(reader)
        if err == io.EOF {
            break
        }
        if err != nil {
            return nil, err
        }
        chunkSize, err := readUint32(reader)
        if err != nil {
            return nil, err
        }

        switch chunkID {
        case "fmt ":
            if chunkSize < 16 {
                return nil, errors.New("invalid fmt chunk")
            }
            audioFormat, err := readUint16(reader)
            if err != nil {
                return nil, err
            }
            channels, err = readUint16(reader)
            if err != nil {
                return nil, err
            }
            sampleRate, err = readUint32(reader)
            if err != nil {
                return nil, err
            }
            if _, err := readUint32(reader); err != nil {
                return nil, err
            }
            if _, err := readUint16(reader); err != nil {
                return nil, err
            }
            bitsPerSample, err = readUint16(reader)
            if err != nil {
                return nil, err
            }
            if audioFormat != 1 {
                return nil, errors.New("only PCM wav supported")
            }
            remaining := int(chunkSize) - 16
            if remaining > 0 {
                if _, err := io.CopyN(io.Discard, reader, int64(remaining)); err != nil {
                    return nil, err
                }
            }
            fmtFound = true
        case "data":
            if !fmtFound {
                if _, err := io.CopyN(io.Discard, reader, int64(chunkSize)); err != nil {
                    return nil, err
                }
                continue
            }
            if bitsPerSample != 16 {
                return nil, errors.New("only 16-bit PCM supported")
            }
            samples := int(chunkSize / 2)
            pcmData = make([]int16, samples)
            if err := binary.Read(reader, binary.LittleEndian, pcmData); err != nil {
                return nil, err
            }
            dataFound = true
        default:
            if _, err := io.CopyN(io.Discard, reader, int64(chunkSize)); err != nil {
                return nil, err
            }
        }

        if chunkSize%2 == 1 {
            if _, err := reader.ReadByte(); err != nil {
                return nil, err
            }
        }
    }

    if !fmtFound || !dataFound {
        return nil, errors.New("missing fmt or data chunk")
    }

    return &PCM{
        SampleRate: int(sampleRate),
        Channels:   int(channels),
        Data:       pcmData,
    }, nil
}

func WriteWav(path string, pcm *PCM) error {
    if pcm.SampleRate <= 0 || pcm.Channels <= 0 {
        return errors.New("invalid pcm metadata")
    }
    if len(pcm.Data) == 0 {
        return errors.New("empty pcm data")
    }

    file, err := os.Create(path)
    if err != nil {
        return err
    }
    defer file.Close()

    dataSize := uint32(len(pcm.Data) * 2)
    riffSize := 4 + (8 + 16) + (8 + dataSize)

    writer := bufio.NewWriter(file)
    if _, err := writer.WriteString("RIFF"); err != nil {
        return err
    }
    if err := binary.Write(writer, binary.LittleEndian, uint32(riffSize)); err != nil {
        return err
    }
    if _, err := writer.WriteString("WAVE"); err != nil {
        return err
    }
    if _, err := writer.WriteString("fmt "); err != nil {
        return err
    }
    if err := binary.Write(writer, binary.LittleEndian, uint32(16)); err != nil {
        return err
    }
    if err := binary.Write(writer, binary.LittleEndian, uint16(1)); err != nil {
        return err
    }
    if err := binary.Write(writer, binary.LittleEndian, uint16(pcm.Channels)); err != nil {
        return err
    }
    if err := binary.Write(writer, binary.LittleEndian, uint32(pcm.SampleRate)); err != nil {
        return err
    }
    byteRate := uint32(pcm.SampleRate * pcm.Channels * 2)
    if err := binary.Write(writer, binary.LittleEndian, byteRate); err != nil {
        return err
    }
    blockAlign := uint16(pcm.Channels * 2)
    if err := binary.Write(writer, binary.LittleEndian, blockAlign); err != nil {
        return err
    }
    if err := binary.Write(writer, binary.LittleEndian, uint16(16)); err != nil {
        return err
    }
    if _, err := writer.WriteString("data"); err != nil {
        return err
    }
    if err := binary.Write(writer, binary.LittleEndian, dataSize); err != nil {
        return err
    }
    if err := binary.Write(writer, binary.LittleEndian, pcm.Data); err != nil {
        return err
    }
    return writer.Flush()
}

func readChunkID(reader *bufio.Reader) (string, error) {
    data := make([]byte, 4)
    if _, err := io.ReadFull(reader, data); err != nil {
        return "", err
    }
    return string(data), nil
}

func readString(reader *bufio.Reader, length int, expected string) error {
    data := make([]byte, length)
    if _, err := io.ReadFull(reader, data); err != nil {
        return err
    }
    if string(data) != expected {
        return errors.New("invalid wav header")
    }
    return nil
}

func readUint16(reader *bufio.Reader) (uint16, error) {
    var value uint16
    err := binary.Read(reader, binary.LittleEndian, &value)
    return value, err
}

func readUint32(reader *bufio.Reader) (uint32, error) {
    var value uint32
    err := binary.Read(reader, binary.LittleEndian, &value)
    return value, err
}
