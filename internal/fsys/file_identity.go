package fsys

import "os"

// SameFileIdentity compares the device/inode identity supplied by OSFS or Fake.
// Missing or unsupported identities refuse rather than assuming sameness.
func SameFileIdentity(left, right os.FileInfo) bool {
	if left == nil || right == nil {
		return false
	}
	leftID, leftOK := fileIdentityFromInfo(left)
	rightID, rightOK := fileIdentityFromInfo(right)
	return leftOK && rightOK && leftID == rightID
}
