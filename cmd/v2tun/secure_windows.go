//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// Only SYSTEM and Administrators may read or change these files: they hold
// server credentials (UUIDs, passwords, subscription URLs). The app always
// runs elevated, so it keeps full access; other users of the PC have none.
const secretSDDL = "D:P(A;;FA;;;SY)(A;;FA;;;BA)"

func protectFile(path string) error {
	sd, err := windows.SecurityDescriptorFromString(secretSDDL)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

// writeSecret writes a settings / credentials file and locks its permissions.
func writeSecret(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	if os.Getenv("V2TUN_DATADIR") == "" { // tests run unelevated in a temp dir
		protectFile(path)
	}
	return nil
}
