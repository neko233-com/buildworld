//go:build windows

package cli

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestWindowsAutostartUsesHiddenPowerShellLauncher(t *testing.T) {
	server := `C:\Program Files\Build'World\buildworld-server.exe`
	config := `C:\Users\Example User\AppData\Roaming\buildworld\config.yaml`
	script := `C:\Users\Example User\AppData\Roaming\buildworld\autostart.ps1`
	action, err := windowsAutostartCommand(script)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(action, "-WindowStyle Hidden") || !strings.HasSuffix(action, `-File "`+script+`"`) {
		t.Fatalf("autostart action = %q", action)
	}
	if strings.Contains(action, `\"`) {
		t.Fatalf("autostart action contains literal backslash quotes: %q", action)
	}

	command := decodePowerShellScript(t, windowsAutostartScript(server, config))
	want := "$ErrorActionPreference = 'Stop'\r\n& 'C:\\Program Files\\Build''World\\buildworld-server.exe' -config 'C:\\Users\\Example User\\AppData\\Roaming\\buildworld\\config.yaml' >> 'C:\\Users\\Example User\\AppData\\Roaming\\buildworld\\server.log' 2>&1\r\nexit $LASTEXITCODE\r\n"
	if command != want {
		t.Fatalf("decoded PowerShell script = %q, want %q", command, want)
	}
}

func TestWindowsAutostartCommandRejectsSchtasksOverflow(t *testing.T) {
	_, err := windowsAutostartCommand(`C:\` + strings.Repeat("long-directory\\", 20) + autostartScriptName)
	if err == nil || !strings.Contains(err.Error(), "schtasks limit") {
		t.Fatalf("overflow error = %v", err)
	}
}

func decodePowerShellScript(t *testing.T, data []byte) string {
	t.Helper()
	if len(data) < 2 || binary.LittleEndian.Uint16(data) != 0xfeff {
		t.Fatal("PowerShell script is missing UTF-16LE BOM")
	}
	data = data[2:]
	if len(data)%2 != 0 {
		t.Fatalf("encoded command has odd UTF-16 byte length: %d", len(data))
	}
	values := make([]uint16, len(data)/2)
	for index := range values {
		values[index] = binary.LittleEndian.Uint16(data[index*2:])
	}
	return string(utf16.Decode(values))
}
