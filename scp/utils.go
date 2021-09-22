/**
 * @Author   DenysGeng <cnphp@hotmail.com>
 *
 * @Description //TODO
 * @Version: 1.0.0
 * @Date     2021/9/22
 */

package scp

import "io"

// An adaptation of io.CopyN that keeps reading if it did not return
// a sufficient amount of bytes.
func CopyN(writer io.Writer, src io.Reader, size int64) (int64, error) {
	var total int64
	total = 0
	for total < size {
		n, err := io.CopyN(writer, src, size)
		if err != nil {
			return 0, err
		}
		total += n
	}

	return total, nil
}
