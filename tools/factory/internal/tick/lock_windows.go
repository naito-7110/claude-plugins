//go:build windows

package tick

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// tryLock は path の排他ロックを非ブロッキングで取得する(LockFileEx)。
// busy = 他プロセスが保持中(正常系)。
// release はロックの解放(ファイルは残る — 再利用される)。
func tryLock(path string) (release func(), busy bool, err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, false, err
	}
	overlapped := new(windows.Overlapped)
	err = windows.LockFileEx(windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, overlapped)
	if err != nil {
		_ = f.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, true, nil
		}
		return nil, false, err
	}
	return func() {
		_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, new(windows.Overlapped))
		_ = f.Close()
	}, false, nil
}
