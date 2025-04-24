// Command-line tool to generate an audio sprite from WAV files and export a JSON map.
// Usage: go-sprite -o sfx-sprite -loops attack.wav,explosion.wav input1.wav input2.wav ...
// Supports glob patterns (e.g. "*.wav").

package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "log"
    "os"
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
    outBase := flag.String("o", "sprite", "Output base name (without extension)")
    loopList := flag.String("loops", "", "Comma-separated list of filenames to loop by default")
    flag.Parse()

    // Collect and expand input patterns
    var inputs []string
    for _, pattern := range flag.Args() {
        matched, err := filepath.Glob(pattern)
        if err != nil {
            log.Fatalf("Invalid pattern %s: %v", pattern, err)
        }
        if len(matched) == 0 {
            log.Fatalf("No files matched pattern: %s", pattern)
        }
        inputs = append(inputs, matched...)
    }
    if len(inputs) == 0 {
        flag.Usage()
        os.Exit(1)
    }

    // Parse loop filenames into a set
    loops := make(map[string]bool)
    if *loopList != "" {
        for _, name := range strings.Split(*loopList, ",") {
            trimmed := strings.TrimSpace(name)
            if trimmed != "" {
                loops[trimmed] = true
            }
        }
    }

    // Combine WAV files
    var outBuf *audio.IntBuffer
    var sampleRate int
    currentSample := 0
    spritemap := make(map[string]SpriteMapEntry)

    for _, infile := range inputs {
        f, err := os.Open(infile)
        if err != nil {
            log.Fatalf("Failed to open %s: %v", infile, err)
        }
        decoder := wav.NewDecoder(f)
        if !decoder.IsValidFile() {
            log.Fatalf("%s is not a valid WAV file", infile)
        }
        buf, err := decoder.FullPCMBuffer()
        f.Close()
        if err != nil {
            log.Fatalf("Failed to decode %s: %v", infile, err)
        }

        if outBuf == nil {
            sampleRate = buf.Format.SampleRate
            outBuf = &audio.IntBuffer{
                Format:         buf.Format,
                Data:           []int{},
                SourceBitDepth: buf.SourceBitDepth,
            }
        } else if buf.Format.SampleRate != sampleRate {
            log.Fatalf("Sample rate mismatch: %s has %d, expected %d", infile, buf.Format.SampleRate, sampleRate)
        }

        start := float64(currentSample) / float64(sampleRate)
        outBuf.Data = append(outBuf.Data, buf.Data...)
        currentSample += len(buf.Data) / buf.Format.NumChannels
        end := float64(currentSample) / float64(sampleRate)

        key := fileKey(infile)
        spritemap[key] = SpriteMapEntry{
            Start: start,
            End:   end,
            Loop:  loops[filepath.Base(infile)],
        }
    }

    // Write combined WAV
    outWav := *outBase + ".wav"
    wf, err := os.Create(outWav)
    if err != nil {
        log.Fatalf("Failed to create output WAV: %v", err)
    }
    encoder := wav.NewEncoder(wf, sampleRate, outBuf.SourceBitDepth, outBuf.Format.NumChannels, 1)
    if err := encoder.Write(outBuf); err != nil {
        log.Fatalf("Failed to write samples: %v", err)
    }
    encoder.Close()
    wf.Close()

    // Write JSON
    sprite := SpriteJSON{
        Resources: []string{outWav},
        Spritemap: spritemap,
    }
    jsonData, err := json.MarshalIndent(sprite, "", "  ")
    if err != nil {
        log.Fatalf("Failed to marshal JSON: %v", err)
    }
    outJson := *outBase + ".json"
    if err := os.WriteFile(outJson, jsonData, 0644); err != nil {
        log.Fatalf("Failed to write JSON file: %v", err)
    }

    fmt.Printf("Generated %s and %s\n", outWav, outJson)
}

func fileKey(path string) string {
    base := filepath.Base(path)
    ext := filepath.Ext(base)
    return base[:len(base)-len(ext)]
}
