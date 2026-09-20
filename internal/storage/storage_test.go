package storage

import (
	"errors"
	"testing"
)

func TestParseBytes(t *testing.T) {
	tests := []struct {
		input    string
		expected uint64
		wantErr  bool
	}{
		{"", 0, false},
		{"0", 0, false},
		{"1024", 1024, false},
		{"1024B", 1024, false},
		{"1K", 1024, false},
		{"1KB", 1024, false},
		{"500M", 500 * 1024 * 1024, false},
		{"500MB", 500 * 1024 * 1024, false},
		{"2G", 2 * 1024 * 1024 * 1024, false},
		{"2GB", 2 * 1024 * 1024 * 1024, false},
		{"1.5G", uint64(1.5 * 1024 * 1024 * 1024), false},
		{"10GiB", 10 * 1024 * 1024 * 1024, false},
		{"invalid", 0, true},
		{"-5G", 0, true},
	}

	for _, tc := range tests {
		got, err := ParseBytes(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseBytes(%q) expected error, got nil", tc.input)
			}
		} else {
			if err != nil {
				t.Errorf("ParseBytes(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.expected {
				t.Errorf("ParseBytes(%q) = %d, want %d", tc.input, got, tc.expected)
			}
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    uint64
		expected string
	}{
		{500, "500 B"},
		{1024, "1.00 KB"},
		{1048576, "1.00 MB"},
		{1073741824, "1.00 GB"},
		{2147483648, "2.00 GB"},
	}

	for _, tc := range tests {
		res := FormatBytes(tc.bytes)
		if res != tc.expected {
			t.Errorf("FormatBytes(%d) = %q, want %q", tc.bytes, res, tc.expected)
		}
	}
}

func TestMockDiskChecker_CheckFreeSpace(t *testing.T) {
	// Total 100GB, Free 5GB
	total := uint64(100 * 1024 * 1024 * 1024)
	free := uint64(5 * 1024 * 1024 * 1024)
	mock := NewMockDiskChecker(total, free)

	// 1. Min free 2GB -> Should pass (5GB >= 2GB)
	min2G := uint64(2 * 1024 * 1024 * 1024)
	if err := mock.CheckFreeSpace("/tmp", min2G); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}

	// 2. Min free 10GB -> Should fail with ErrInsufficientDiskSpace (5GB < 10GB)
	min10G := uint64(10 * 1024 * 1024 * 1024)
	err := mock.CheckFreeSpace("/tmp", min10G)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, ErrInsufficientDiskSpace) {
		t.Errorf("expected ErrInsufficientDiskSpace, got %v", err)
	}
}

func TestMockDiskChecker_CheckTaskSpace(t *testing.T) {
	// Total 100GB, Free 5GB, MinFree 2GB (Safe to allocate up to 3GB)
	total := uint64(100 * 1024 * 1024 * 1024)
	free := uint64(5 * 1024 * 1024 * 1024)
	mock := NewMockDiskChecker(total, free)
	minFree := uint64(2 * 1024 * 1024 * 1024)

	// Task requires 2GB -> Should pass (5 - 2 >= 2)
	req2G := uint64(2 * 1024 * 1024 * 1024)
	if err := mock.CheckTaskSpace("/tmp", req2G, minFree); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}

	// Task requires 4GB -> Should fail (5 - 4 < 2)
	req4G := uint64(4 * 1024 * 1024 * 1024)
	err := mock.CheckTaskSpace("/tmp", req4G, minFree)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, ErrInsufficientDiskSpace) {
		t.Errorf("expected ErrInsufficientDiskSpace, got %v", err)
	}
}

func TestOSDiskChecker_RealSystem(t *testing.T) {
	checker := NewOSDiskChecker()
	usage, err := checker.GetDiskUsage("/tmp")
	if err != nil {
		t.Fatalf("GetDiskUsage failed: %v", err)
	}
	if usage.TotalBytes == 0 {
		t.Errorf("expected TotalBytes > 0, got 0")
	}
	if usage.FreeBytes == 0 {
		t.Errorf("expected FreeBytes > 0, got 0")
	}

	// Checking 1 Byte free should almost always pass on /tmp
	if err := checker.CheckFreeSpace("/tmp", 1); err != nil {
		t.Errorf("CheckFreeSpace(1) failed: %v", err)
	}
}
