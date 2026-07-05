//go:build !windows

package tick

import (
	"errors"
	"os"
	"syscall"
)

// tryLock は path のアドバイザリロックを非ブロッキングで取得する。
// busy = 他プロセス(または他の tick)が保持中(正常系)。
// release はロックの解放(ファイルは残る — 再利用される)。
//
// flock(2) を直接使う: flock **コマンド**は util-linux 由来で macOS に
// 存在しないが、システムコールは macOS / Linux 共通で使える。
func tryLock(path string) (release func(), busy bool, err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, true, nil
		}
		return nil, false, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, false, nil
}
