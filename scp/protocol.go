/**
 * @Author   DenysGeng <cnphp@hotmail.com>
 *
 * @Description //TODO
 * @Version: 1.0.0
 * @Date     2021/9/22
 */

package scp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
)

type RespType = uint8

const (
	Ok      RespType = 0
	Warning RespType = 1
	Error   RespType = 2
	StreamC RespType = 'C'
	StreamD RespType = 'D'
	StreamE RespType = 'E'
)

type Msg string

func (m Msg) String() string {
	return string(m)
}

func (m Msg) FileInfo() (mode os.FileMode, size int64, filename string, err error) {
	_, err = fmt.Sscanf(string(m), "%04o %d %s", &mode, &size, &filename)
	return
}

type Resp struct {
	Type RespType
	Msg  Msg
}

func ReadResp(reader io.Reader) (*Resp, error) {
	buffer := make([]uint8, 1)
	_, err := reader.Read(buffer)
	if err != nil && err != io.EOF {
		return &Resp{}, err
	}
	msgType := buffer[0]
	msg := ""
	switch msgType {
	case Ok:
	case Warning:
	case Error:
	case StreamC:
	case StreamD:
	case StreamE:
	default:
		return nil, errors.New("invalid protocol " + string(msgType))
	}
	if msgType > Ok {
		rd := bufio.NewReader(reader)
		msg, err = rd.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return &Resp{}, nil
			}
			return nil, err
		}
		msg = msg[:len(msg)-1]
	}

	return &Resp{Type: msgType, Msg: Msg(msg)}, nil
}

func (rsp *Resp) IsOk() bool {
	return rsp.Type == Ok
}

func (rsp *Resp) IsWarning() bool {
	return rsp.Type == Warning
}

func (rsp *Resp) IsError() bool {
	return rsp.Type == Error
}

func (rsp *Resp) IsFailure() bool {
	return rsp.Type == 1 || rsp.Type == 2
}

func (rsp *Resp) GetMessage() Msg {
	return rsp.Msg
}

func (rsp *Resp) IsFile() bool {
	return rsp.Type == StreamC
}

func (rsp *Resp) IsDir() bool {
	return rsp.Type == StreamD
}

func (rsp *Resp) IsEndDir() bool {
	return rsp.Type == StreamE
}

func (rsp *Resp) Write(w io.Writer) error {
	_, err := fmt.Fprint(w, fmt.Sprintf("%s%s", string(rsp.Type), rsp.Msg))
	if err != nil {
		return err
	}
	if rsp.Type != Ok {
		_, err = fmt.Fprintln(w, "")
		if err != nil {
			return err
		}
	}

	return nil
}

func (rsp *Resp) WriteStream(w io.Writer, stream io.Reader) error {
	err := rsp.Write(w)
	if err != nil {
		return err
	}

	_, err = io.Copy(w, stream)
	if err != nil {
		return err
	}

	_, err = fmt.Fprint(w, "\x00")
	if err != nil {
		return err
	}

	return nil
}

func NewOkRsp() *Resp {
	return &Resp{}
}

func NewErrorRsp(msg string) *Resp {
	return &Resp{Type: Error, Msg: Msg(msg)}
}

func NewWarnRsp(msg string) *Resp {
	return &Resp{Type: Warning, Msg: Msg(msg)}
}

func NewDirBegin(mode os.FileMode, dirname string) *Resp {
	return &Resp{
		Type: StreamD,
		Msg:  Msg(fmt.Sprintf("%04o 0 %s", mode&os.ModePerm, dirname)),
	}
}

func NewDirEnd() *Resp {
	return &Resp{
		Type: StreamE,
		Msg:  "",
	}
}

func NewFile(mode os.FileMode, filename string, size int64) *Resp {
	return &Resp{
		Type: StreamC,
		Msg:  Msg(fmt.Sprintf("%04o %d %s", mode&os.ModePerm, size, filename)),
	}
}