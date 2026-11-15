package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Fever-r/BombeCam/pkg/bridge"
	"github.com/Fever-r/BombeCam/pkg/profile"
)

func waitCameraEvent(t *testing.T, events <-chan string, want string) {
	t.Helper()
	select {
	case got := <-events:
		if got != want {
			t.Fatalf("camera event = %q, want %q", got, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", want)
	}
}

func TestCameraStreamControls(t *testing.T) {
	sm := NewStreamManager(nil, "", "", nil, false, "ffmpeg")
	defer sm.CloseAll()
	started, stopped := make(chan string, 10), make(chan string, 10)
	sm.SetRunner(func(ctx context.Context, mc *ManagedCamera, _ bridge.Device, _ int) {
		started <- mc.UUID
		<-ctx.Done()
		stopped <- mc.UUID
	})
	devices := []bridge.Device{{UUID: "one"}, {UUID: "two"}}
	if _, err := sm.enroll([]string{"one", "two"}, devices, map[string]bool{"one": true}); err != nil {
		t.Fatal(err)
	}
	waitCameraEvent(t, started, "two")
	if !sm.IsStopped("one") || sm.IsStopped("two") {
		t.Fatal("restored paused camera should not start, and the other should run")
	}
	sm.StartStreaming()
	if !sm.IsStopped("one") {
		t.Fatal("global start resumed an individually paused camera")
	}
	sm.SetCameraPaused("one", false)
	waitCameraEvent(t, started, "one")
	sm.SetCameraPaused("one", true)
	waitCameraEvent(t, stopped, "one")
	if sm.IsStopped("two") {
		t.Fatal("stopping one camera stopped the other")
	}
	sm.StopStreaming()
	waitCameraEvent(t, stopped, "two")
	sm.StartStreaming()
	waitCameraEvent(t, started, "two")
	if !sm.IsStopped("one") {
		t.Fatal("global stop/start lost the individual pause")
	}
	sm.StopStreaming()
	waitCameraEvent(t, stopped, "two")
	sm.SetCameraPaused("one", false)
	waitCameraEvent(t, started, "one")
	sm.RestartActiveStreaming()
	waitCameraEvent(t, stopped, "one")
	waitCameraEvent(t, started, "one")
	if !sm.AllStopped() || !sm.IsStopped("two") {
		t.Fatal("refreshing active publishers changed global stop state or started another camera")
	}
}

func TestCameraStreamRapidRestartWaitsForCleanup(t *testing.T) {
	sm := NewStreamManager(nil, "", "", nil, false, "ffmpeg")
	defer sm.CloseAll()
	started, exiting := make(chan string, 2), make(chan string, 2)
	release := make(chan struct{})
	sm.SetRunner(func(ctx context.Context, mc *ManagedCamera, _ bridge.Device, _ int) {
		started <- mc.UUID
		<-ctx.Done()
		exiting <- mc.UUID
		<-release
	})
	if _, err := sm.Enroll([]string{"one"}, []bridge.Device{{UUID: "one"}}); err != nil {
		t.Fatal(err)
	}
	waitCameraEvent(t, started, "one")
	sm.SetCameraPaused("one", true)
	waitCameraEvent(t, exiting, "one")
	sm.SetCameraPaused("one", false)
	sm.SetCameraPaused("one", true)
	sm.SetCameraPaused("one", false)
	select {
	case <-started:
		t.Fatal("replacement started before previous runner finished cleanup")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	waitCameraEvent(t, started, "one")
}

type failingCameraStreamStore struct{ profile.ProfileManager }

func (p failingCameraStreamStore) Update(context.Context, func(*profile.Profile) error) (*profile.Profile, error) {
	return nil, errors.New("synthetic save failure")
}

func TestCameraStreamTogglePersistence(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "profile.enc")
	key := []byte("01234567890123456789012345678901")
	pm := profile.NewManager(path, key)
	if err := pm.Save(ctx, &profile.Profile{Cameras: map[string]profile.CameraProfile{
		"one": {UUID: "one"}, "two": {UUID: "two"},
	}}); err != nil {
		t.Fatal(err)
	}
	sm := NewStreamManager(nil, "", "", nil, false, "ffmpeg")
	defer sm.CloseAll()
	sm.SetRunner(func(ctx context.Context, _ *ManagedCamera, _ bridge.Device, _ int) { <-ctx.Done() })
	if _, err := sm.Enroll([]string{"one", "two"}, []bridge.Device{{UUID: "one"}, {UUID: "two"}}); err != nil {
		t.Fatal(err)
	}
	oldOperator := GatewayOperatorManager()
	SetGatewayOperatorManager(nil)
	defer SetGatewayOperatorManager(oldOperator)
	toggle := func(id string, paused bool, store profile.ProfileManager) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		action := "start"
		if paused {
			action = "stop"
		}
		SetupAPIMux(nil, sm, store).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/cameras/"+id+"/stream/"+action, nil))
		return rec
	}
	if rec := toggle("one", true, failingCameraStreamStore{pm}); rec.Code != http.StatusInternalServerError || sm.IsStopped("one") {
		t.Fatalf("save failure changed runtime or returned %d: %s", rec.Code, rec.Body.String())
	}
	if rec := toggle("missing", true, pm); rec.Code != http.StatusNotFound {
		t.Fatalf("missing camera returned %d", rec.Code)
	}
	if rec := toggle("one", true, pm); rec.Code != http.StatusOK || !sm.IsStopped("one") || sm.IsStopped("two") {
		t.Fatalf("stop failed: %d %s", rec.Code, rec.Body.String())
	}
	saved, err := profile.NewManager(path, key).Load(ctx)
	if err != nil || !saved.Cameras["one"].StreamPaused || saved.Cameras["two"].StreamPaused {
		t.Fatalf("paused state was not saved correctly: %+v, %v", saved, err)
	}
	if rec := toggle("one", false, pm); rec.Code != http.StatusOK || sm.IsStopped("one") {
		t.Fatalf("start failed: %d %s", rec.Code, rec.Body.String())
	}
	saved, err = profile.NewManager(path, key).Load(ctx)
	if err != nil || saved.Cameras["one"].StreamPaused {
		t.Fatalf("resumed state was not saved: %+v, %v", saved, err)
	}
}
