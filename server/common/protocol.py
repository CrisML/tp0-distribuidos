import socket
import struct

MSG_BET = 0x01
MSG_ACK = 0x02
MSG_BATCH = 0x03

ACK_OK = 0x00
ACK_ERROR = 0x01


def recv_exact(sock: socket.socket, n: int) -> bytes:
    data = b""
    while len(data) < n:
        chunk = sock.recv(n - len(data))
        if chunk == b"":
            raise ConnectionError("socket closed")
        data += chunk
    return data


def recv_frame(sock: socket.socket) -> bytes:
    hdr = recv_exact(sock, 4)
    (length,) = struct.unpack("!I", hdr)
    if length <= 0:
        raise ValueError("invalid frame length")
    return recv_exact(sock, length)


def send_frame(sock: socket.socket, payload: bytes) -> None:
    hdr = struct.pack("!I", len(payload))
    sock.sendall(hdr + payload)


def _read_u16(buf: bytes, off: int) -> tuple[int, int]:
    if off + 2 > len(buf):
        raise ValueError("truncated u16")
    (v,) = struct.unpack("!H", buf[off : off + 2])
    return v, off + 2


def _read_u32(buf: bytes, off: int) -> tuple[int, int]:
    if off + 4 > len(buf):
        raise ValueError("truncated u32")
    (v,) = struct.unpack("!I", buf[off : off + 4])
    return v, off + 4


def _read_u8(buf: bytes, off: int) -> tuple[int, int]:
    if off + 1 > len(buf):
        raise ValueError("truncated u8")
    return buf[off], off + 1


def _read_str(buf: bytes, off: int) -> tuple[str, int]:
    ln, off = _read_u16(buf, off)
    if off + ln > len(buf):
        raise ValueError("truncated str")
    s = buf[off : off + ln].decode("utf-8")
    return s, off + ln


def _read_bet_body(buf: bytes, off: int) -> tuple[dict, int]:
    agency, off = _read_u8(buf, off)
    first_name, off = _read_str(buf, off)
    last_name, off = _read_str(buf, off)
    document, off = _read_str(buf, off)
    birthdate, off = _read_str(buf, off)
    number, off = _read_u32(buf, off)
    return {
        "agency": agency,
        "first_name": first_name,
        "last_name": last_name,
        "document": document,
        "birthdate": birthdate,
        "number": number,
    }, off


def decode_bet(payload: bytes) -> dict:
    off = 0
    msg_type, off = _read_u8(payload, off)
    if msg_type != MSG_BET:
        raise ValueError("unexpected msg type")

    agency, off = _read_u8(payload, off)
    first_name, off = _read_str(payload, off)
    last_name, off = _read_str(payload, off)
    document, off = _read_str(payload, off)
    birthdate, off = _read_str(payload, off)
    number, off = _read_u32(payload, off)

    if off != len(payload):
        raise ValueError("extra bytes in payload")

    return {
        "agency": agency,
        "first_name": first_name,
        "last_name": last_name,
        "document": document,
        "birthdate": birthdate,
        "number": number,
    }


def decode_message(payload: bytes) -> dict:
    off = 0
    msg_type, off = _read_u8(payload, off)

    if msg_type == MSG_BET:
        bet, off = _read_bet_body(payload, off)
        if off != len(payload):
            raise ValueError("extra bytes in payload")
        return {"type": "BET", "bets": [bet]}

    if msg_type == MSG_BATCH:
        cnt, off = _read_u16(payload, off)
        bets = []
        for _ in range(cnt):
            bet, off = _read_bet_body(payload, off)
            bets.append(bet)
        if off != len(payload):
            raise ValueError("extra bytes in payload")
        return {"type": "BATCH", "bets": bets}

    raise ValueError("unknown msg type")


def encode_ack(ok: bool) -> bytes:
    status = ACK_OK if ok else ACK_ERROR
    return bytes([MSG_ACK, status])