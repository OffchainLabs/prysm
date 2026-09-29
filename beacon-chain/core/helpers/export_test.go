package helpers

// WaitForSyncCommitteeCacheFills blocks until all in-flight sync committee cache fills are done.
func WaitForSyncCommitteeCacheFills() {
	pendingSyncCommitteeCacheFills.Wait()
}
