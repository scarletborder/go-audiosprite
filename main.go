package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
)

type SpriteMapEntry struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Loop  bool    `json:"loop"`
}

type SpriteJSON struct {
	Resources []string                  `json:"resources"`
	Spritemap map[string]SpriteMapEntry `json:"spritemap"`
}

func main() {
	outBase := flag.String("o", "sprite", "输出文件基名（不含扩展名）")
	loopList := flag.String("loops", "", "默认循环的文件名列表，用逗号分隔")
	formatFlag := flag.String("format", "wav", "输出音频格式，可选: wav, mp3, ogg")
	flag.Parse()

	// 检查格式合法性
	valid := map[string]bool{"wav": true, "mp3": true, "ogg": true}
	if !valid[strings.ToLower(*formatFlag)] {
		log.Fatalf("不支持的格式: %s，仅支持 wav, mp3, ogg", *formatFlag)
	}

	var inputs []string
	for _, pattern := range flag.Args() {
		matched, err := filepath.Glob(pattern)
		if err != nil {
			log.Fatalf("无效的模式 %s: %v", pattern, err)
		}
		if len(matched) == 0 {
			log.Fatalf("没有匹配到任何文件: %s", pattern)
		}
		inputs = append(inputs, matched...)
	}
	if len(inputs) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	loops := make(map[string]bool)
	if *loopList != "" {
		for _, name := range strings.Split(*loopList, ",") {
			loops[strings.TrimSpace(name)] = true
		}
	}

	var outBuf *audio.IntBuffer
	var targetRate int
	currentSample := 0
	spritemap := make(map[string]SpriteMapEntry)

	for _, infile := range inputs {
		buf, err := decodeWAV(infile)
		if err != nil {
			log.Fatalf("解码 %s 失败: %v", infile, err)
		}
		if outBuf == nil {
			targetRate = buf.Format.SampleRate
			outBuf = &audio.IntBuffer{
				Format:         buf.Format,
				Data:           []int{},
				SourceBitDepth: buf.SourceBitDepth,
			}
		} else if buf.Format.SampleRate != targetRate {
			tmpResampled, err := ffmpegResample(infile, targetRate)
			if err != nil {
				log.Fatalf("重采样 %s 失败: %v", infile, err)
			}
			defer os.Remove(tmpResampled)
			buf, err = decodeWAV(tmpResampled)
			if err != nil {
				log.Fatalf("解码重采样文件 %s 失败: %v", tmpResampled, err)
			}
		}

		start := float64(currentSample) / float64(targetRate)
		outBuf.Data = append(outBuf.Data, buf.Data...)
		currentSample += len(buf.Data) / buf.Format.NumChannels
		end := float64(currentSample) / float64(targetRate)

		key := fileKey(infile)
		spritemap[key] = SpriteMapEntry{
			Start: start,
			End:   end,
			Loop:  loops[filepath.Base(infile)],
		}
	}

	// 临时 WAV 输出
	tmpWav := *outBase + ".wav"
	writeWAV(tmpWav, outBuf, targetRate)

	// 如果目标格式不是 wav，则转换
	outAudio := *outBase + "." + strings.ToLower(*formatFlag)
	if strings.ToLower(*formatFlag) != "wav" {
		if err := ffmpegConvert(tmpWav, outAudio, *formatFlag); err != nil {
			log.Fatalf("转换 %s 失败: %v", outAudio, err)
		}
		os.Remove(tmpWav)
	} else {
		outAudio = tmpWav
	}

	// 写出 JSON
	sprite := SpriteJSON{
		Resources: []string{outAudio},
		Spritemap: spritemap,
	}
	data, _ := json.MarshalIndent(sprite, "", "  ")
	if err := ioutil.WriteFile(*outBase+".json", data, 0644); err != nil {
		log.Fatalf("写入 JSON 失败: %v", err)
	}

	fmt.Printf("生成 %s 和 %s 完成\n", outAudio, *outBase+".json")
}

func decodeWAV(path string) (*audio.IntBuffer, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := wav.NewDecoder(f)
	if !dec.IsValidFile() {
		return nil, fmt.Errorf("%s 不是有效 WAV", path)
	}
	return dec.FullPCMBuffer()
}

func writeWAV(path string, buf *audio.IntBuffer, sampleRate int) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("创建输出文件失败: %v", err)
	}
	defer f.Close()
	enc := wav.NewEncoder(f, sampleRate, buf.SourceBitDepth, buf.Format.NumChannels, 1)
	if err := enc.Write(buf); err != nil {
		log.Fatalf("写入 WAV 失败: %v", err)
	}
	enc.Close()
}

func ffmpegResample(input string, rate int) (string, error) {
	tmp := fmt.Sprintf("%s_resampled_%d.wav", input, rate)
	cmd := exec.Command("ffmpeg", "-y", "-i", input,
		"-ar", fmt.Sprint(rate), tmp)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg error: %v, %s", err, string(out))
	}
	return tmp, nil
}

func ffmpegConvert(input, output, format string) error {
	args := []string{"-y", "-i", input}
	// 自动选择编码器
	if strings.ToLower(format) == "mp3" {
		args = append(args, "-codec:a", "libmp3lame", "-qscale:a", "2")
	} else if strings.ToLower(format) == "ogg" {
		args = append(args, "-codec:a", "libvorbis")
	}
	args = append(args, output)
	cmd := exec.Command("ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg convert error: %v, %s", err, string(out))
	}
	return nil
}

func fileKey(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return base[:len(base)-len(ext)]
}
