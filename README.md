# go-audiosprite

make for my own HTML5 game project. 

currently only support `.wav`,`.ogg`,`.mp3`

## prerequisites

`ffmpeg`



## usage

```bash
# 把当前目录下所有 wav 文件合并并导出
./go-audiosprite -o sfx-sprite *.wav

# 指定循环片段
./go-audiosprite -o sfx-sprite -loops attack.wav,zombie.wav sounds/*.wav
```