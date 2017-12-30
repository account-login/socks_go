package util

import "io"

func ReadRequired(reader io.Reader, n int) (data []byte, err error) {
	data = make([]byte, n)
	_, err = io.ReadFull(reader, data)
	return
}

func BridgeReaderWriter(reader io.Reader, writer io.Writer) (errchan chan error) {
	errchan = make(chan error, 2)
	buf := make([]byte, 4096)
	go func() {
		for {
			n, err := reader.Read(buf)
			var werr error
			if n > 0 {
				_, werr = writer.Write(buf[:n])
			}

			if err != nil || werr != nil {
				rerr := err
				if rerr == io.EOF {
					rerr = nil
				}

				errchan <- rerr
				errchan <- werr
				return
			}
		}
	}()
	return
}
