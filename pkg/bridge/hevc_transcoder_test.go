package bridge

import (
	"slices"
	"testing"
)

func TestBuildHEVCTranscoderArgsKeepsExistingOutputCodecs(t *testing.T) {
	const rtspURL = "rtsp://127.0.0.1:8554/camera"
	const audioURL = "tcp://127.0.0.1:45678"

	withAudio := buildHEVCTranscoderArgs(true, audioURL, rtspURL)
	for _, want := range []string{"-f", "hevc", "-i", "pipe:0", "-f", "aac", "-i", audioURL, "libx264", "-rtsp_transport", "tcp", rtspURL} {
		if !slices.Contains(withAudio, want) {
			t.Errorf("audio HEVC args missing %q: %v", want, withAudio)
		}
	}
	if slices.Contains(withAudio, "-an") {
		t.Fatalf("audio HEVC args unexpectedly disable audio: %v", withAudio)
	}

	videoOnly := buildHEVCTranscoderArgs(false, "", rtspURL)
	if !slices.Contains(videoOnly, "-an") || slices.Contains(videoOnly, "tcp://") {
		t.Fatalf("video-only HEVC args have wrong audio settings: %v", videoOnly)
	}
	if !slices.Contains(videoOnly, "libx264") || slices.Contains(videoOnly, "h264_vaapi") {
		t.Fatalf("CPU HEVC args do not use libx264: %v", videoOnly)
	}
}

func TestBuildHEVCTranscoderArgsUsesVAAPIWhenAvailable(t *testing.T) {
	const rtspURL = "rtsp://127.0.0.1:8554/camera"
	const audioURL = "tcp://127.0.0.1:45678"
	encoder := hevcVideoEncoder{name: "h264_vaapi", device: "/dev/dri/renderD128"}

	withAudio := buildHEVCTranscoderArgsWithEncoder(true, audioURL, rtspURL, encoder)
	for _, want := range []string{
		"vaapi=va:/dev/dri/renderD128", "-filter_hw_device", "va",
		"-vf", "format=nv12,hwupload", "h264_vaapi", "-qp", "23",
		"-f", "hevc", "-i", "pipe:0", "-f", "aac", "-i", audioURL, "pcm_mulaw",
		"-rtsp_transport", "tcp", rtspURL,
	} {
		if !slices.Contains(withAudio, want) {
			t.Errorf("VAAPI HEVC args missing %q: %v", want, withAudio)
		}
	}
	if slices.Contains(withAudio, "libx264") {
		t.Fatalf("VAAPI HEVC args unexpectedly use the CPU encoder: %v", withAudio)
	}

	videoOnly := buildHEVCTranscoderArgsWithEncoder(false, "", rtspURL, encoder)
	if !slices.Contains(videoOnly, "h264_vaapi") || !slices.Contains(videoOnly, "-an") {
		t.Fatalf("VAAPI video-only args have wrong encoder/audio settings: %v", videoOnly)
	}
}
