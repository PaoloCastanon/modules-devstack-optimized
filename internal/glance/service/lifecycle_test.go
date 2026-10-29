package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStopAdmissionAndDrain(t *testing.T) {
	for _, kind := range []string{"writer", "download", "delete"} {
		t.Run(kind, func(t *testing.T) {
			u, _, _, _ := uploadFixture(t)
			var leave func()
			var err error
			switch kind {
			case "writer":
				leave, err = u.claim(uploadImageID)
			case "download":
				leave, err = u.claimDownload()
			case "delete":
				leave, err = u.claimDeletion(uploadImageID)
			}
			if err != nil {
				t.Fatal(err)
			}
			u.StopAdmission()
			u.StopAdmission()
			if _, err = u.claim("other"); !errors.Is(err, ErrUploadNotReady) {
				t.Fatal("new writer admitted", err)
			}
			if _, err = u.claimDownload(); !errors.Is(err, ErrUploadNotReady) {
				t.Fatal("new reader admitted", err)
			}
			if _, err = u.claimDeletion("other"); !errors.Is(err, ErrUploadNotReady) {
				t.Fatal("new deletion admitted", err)
			}
			if _, err = u.RecoverUploads(context.Background()); !errors.Is(err, ErrUploadNotReady) {
				t.Fatal("recovery reopened admission", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
			defer cancel()
			if err = u.WaitIdle(ctx); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("reported drain while active", err)
			}
			if err = u.Close(); !errors.Is(err, ErrUploadBusy) {
				t.Fatal("released ownership early", err)
			}
			done := make(chan error, 1)
			go func() { done <- u.WaitIdle(context.Background()) }()
			leave()
			select {
			case err = <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("drain not notified")
			}
			if err = u.Close(); err != nil {
				t.Fatal(err)
			}
			if err = u.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
