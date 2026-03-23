package common

import (
    "encoding/binary"
    "fmt"
    "io"
    "net"
)

const (
    msgTypeBet byte = 0x01
    msgTypeAck byte = 0x02

    ackOK    byte = 0x00
    ackError byte = 0x01
)

type Bet struct {
    Agency    uint8
    FirstName string
    LastName  string
    Document  string
    Birthdate string // YYYY-MM-DD
    Number    uint32
}

func SendBet(conn net.Conn, bet Bet) error {
    payload, err := encodeBet(bet)
    if err != nil {
        return err
    }
    if err := writeFrame(conn, payload); err != nil {
        return err
    }

    resp, err := readFrame(conn)
    if err != nil {
        return err
    }
    if len(resp) < 2 || resp[0] != msgTypeAck {
        return fmt.Errorf("invalid ack")
    }
    if resp[1] != ackOK {
        return fmt.Errorf("server returned error")
    }
    return nil
}

func encodeBet(b Bet) ([]byte, error) {
    out := make([]byte, 0, 1+1+2+len(b.FirstName)+2+len(b.LastName)+2+len(b.Document)+2+len(b.Birthdate)+4)

    out = append(out, msgTypeBet)
    out = append(out, byte(b.Agency))

    var err error
    out, err = appendString(out, b.FirstName)
    if err != nil {
        return nil, err
    }
    out, err = appendString(out, b.LastName)
    if err != nil {
        return nil, err
    }
    out, err = appendString(out, b.Document)
    if err != nil {
        return nil, err
    }
    out, err = appendString(out, b.Birthdate)
    if err != nil {
        return nil, err
    }

    var num [4]byte
    binary.BigEndian.PutUint32(num[:], b.Number)
    out = append(out, num[:]...)

    return out, nil
}

func appendString(dst []byte, s string) ([]byte, error) {
    if len(s) > 0xFFFF {
        return nil, fmt.Errorf("string too long")
    }
    var hdr [2]byte
    binary.BigEndian.PutUint16(hdr[:], uint16(len(s)))
    dst = append(dst, hdr[:]...)
    dst = append(dst, []byte(s)...)
    return dst, nil
}

func writeFrame(conn net.Conn, payload []byte) error {
    var hdr [4]byte
    binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
    if err := writeAll(conn, hdr[:]); err != nil {
        return err
    }
    return writeAll(conn, payload)
}

func readFrame(conn net.Conn) ([]byte, error) {
    var hdr [4]byte
    if _, err := io.ReadFull(conn, hdr[:]); err != nil {
        return nil, err
    }
    n := binary.BigEndian.Uint32(hdr[:])
    if n == 0 {
        return nil, fmt.Errorf("invalid frame length")
    }
    buf := make([]byte, n)
    if _, err := io.ReadFull(conn, buf); err != nil {
        return nil, err
    }
    return buf, nil
}

func writeAll(w io.Writer, b []byte) error {
    for len(b) > 0 {
        n, err := w.Write(b)
        if err != nil {
            return err
        }
        b = b[n:]
    }
    return nil
}