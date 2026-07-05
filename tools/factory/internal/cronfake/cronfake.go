// Package cronfake は crontab コマンド(プロセス境界)のインメモリ fake。
// tick.Crontab interface を満たし、テストから注入する(実 crontab に触れない)。
// 最終状態(Content)をアサートする状態検証に使う。
package cronfake

import "errors"

// Crontab はインメモリの crontab。
type Crontab struct {
	Content  string // 現在の crontab の内容
	ReadErr  error  // Read を失敗させたいとき設定する
	WriteErr error  // Write を失敗させたいとき設定する
}

// Read は tick.Crontab を満たす。
func (c *Crontab) Read() (string, error) {
	if c.ReadErr != nil {
		return "", c.ReadErr
	}
	return c.Content, nil
}

// Write は tick.Crontab を満たす。
func (c *Crontab) Write(content string) error {
	if c.WriteErr != nil {
		return c.WriteErr
	}
	c.Content = content
	return nil
}

// Broken は Read / Write を常に失敗させる fake を返す(crontab が使えない環境の再現)。
func Broken() *Crontab {
	err := errors.New("crontab: command not found")
	return &Crontab{ReadErr: err, WriteErr: err}
}
