package common

import (
    "encoding/csv"
    "fmt"
    "io"
    "os"
    "strconv"
    "strings"
)

func ReadBetsFromCSV(path string, agency uint8) ([]Bet, error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer f.Close()

    r := csv.NewReader(f)
    r.Comma = ','
    r.FieldsPerRecord = -1

    var bets []Bet
    for {
        rec, err := r.Read()
        if err == io.EOF {
            break
        }
        if err != nil {
            return nil, err
        }
        if len(rec) == 0 {
            continue
        }
        for i := range rec {
            rec[i] = strings.TrimSpace(rec[i])
        }

        // saltear header
        if strings.Contains(strings.ToLower(rec[0]), "name") || strings.Contains(strings.ToLower(rec[0]), "nombre") {
            continue
        }

        var first, last, doc, birth, numS string
        switch len(rec) {
        case 5:
            first, last, doc, birth, numS = rec[0], rec[1], rec[2], rec[3], rec[4]
        case 6:
            first, last, doc, birth, numS = rec[1], rec[2], rec[3], rec[4], rec[5]
        default:
            return nil, fmt.Errorf("unexpected csv columns: %d", len(rec))
        }

        n, err := strconv.Atoi(numS)
        if err != nil || n < 0 {
            return nil, fmt.Errorf("invalid number: %q", numS)
        }

        bets = append(bets, Bet{
            Agency:    agency,
            FirstName: first,
            LastName:  last,
            Document:  doc,
            Birthdate: birth,
            Number:    uint32(n),
        })
    }

    return bets, nil
}

func ChunkBets(all []Bet, max int) [][]Bet {
    if max <= 0 {
        max = 1
    }
    out := make([][]Bet, 0, (len(all)+max-1)/max)
    for i := 0; i < len(all); i += max {
        j := i + max
        if j > len(all) {
            j = len(all)
        }
        out = append(out, all[i:j])
    }
    return out
}