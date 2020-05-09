# JOY5

AV Toolkit.

### Install

```sh
go get github.com/tuan3w/joy5/cmd/avtool
```

### Benchmark RTMP

```sh
avtool servertmp :1935 movie.flv &
sb_rtmp_load -c 1 -r rtmp://127.0.0.1:1935/live/livestream
```