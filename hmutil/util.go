package hmutil

import (
	// "log"
	// "os/exec"
	"strings"

	"os"
	"path/filepath"
	"bytes"
	"io/ioutil"
	"io"
	"crypto/aes"
	"crypto/cipher"
	"compress/gzip"
	"archive/tar"

	"github.com/rlmcpherson/s3gof3r"
	"github.com/laher/scp-go/scp"
)

// func System(cmd string) ([]byte, error) {
// 	log.Println(cmd)
// 	return exec.Command("sh", "-c", cmd).CombinedOutput()
// }

func ReplaceVars(str string, replacements map[string]string) string {
	for from, to := range replacements {
		str = strings.Replace(str, from, to, -1)
	}
	return str
}

func Tar(dir string, files []string, tw *tar.Writer, prevdir string) error {
	if len(files) == 1 && files[0] == "*" {
		files = []string{}
		d, _err := ioutil.ReadDir(dir)
		
		if _err != nil {
			return _err
		}

		for _, e := range d {
			files = append(files, e.Name())
		}
	}

	for _, file := range files {
		f, _err := os.Open(filepath.Join(dir, file))
		if _err != nil {
			continue
		}

		s, _err := f.Stat()

		if s.IsDir() {
			Tar(filepath.Join(dir, file), []string{"*"}, tw, file)
		} else {
			if _err != nil {
				return _err
			}

			header := &tar.Header{
				Name: filepath.Join(prevdir, s.Name()),
				Size: s.Size(),
				Mode: 0777,
			}

			if _err = tw.WriteHeader(header); _err != nil {
				return _err
			}

			buffer := make([]byte, s.Size())
			buffer, _err = ioutil.ReadFile(filepath.Join(dir, s.Name())) // file.Read(buffer)
			if _err != nil {
				return _err
			}

			if _, _err = tw.Write(buffer); _err != nil {
				return _err
			}
		}
	}

	return nil
}

// func Gzip(pathToFile string, buffer *bytes.Buffer) {
// 	gzFile, _err := os.Create(pathToFile)
// 	handleError(_err)

// 	gzWriter := gzip.NewWriter(gzFile)
// 	_, _err = gzWriter.Write(buffer.Bytes())
// 	handleError(_err)

// 	gzWriter.Close()
// }

func Gzip(buffer *bytes.Buffer) (bytes.Buffer, error) {
	var gzFile bytes.Buffer
	gzWriter := gzip.NewWriter(&gzFile)
	_, _err := gzWriter.Write(buffer.Bytes())
	gzWriter.Flush()
	gzWriter.Close()
	if _err != nil {
		return bytes.Buffer{}, _err
	}
	
	return gzFile, nil
}

func Encode(buffer *bytes.Buffer, encodeKey []byte) (bytes.Buffer, error) {
	var outbuffer bytes.Buffer

	block, _err := aes.NewCipher(encodeKey)
	if _err != nil {
		return bytes.Buffer{}, _err
	}

	var iv [aes.BlockSize]byte
	stream := cipher.NewOFB(block, iv[:])

	writer := &cipher.StreamWriter{S: stream, W: &outbuffer}
	defer writer.Close()
	if _, _err = io.Copy(writer, buffer); _err != nil {
		return bytes.Buffer{}, _err
	}

	return outbuffer, nil
}

func WriteToFile(path string, buffer bytes.Buffer) error {
	file, _err := os.Create(path)
	defer file.Close()
	if _err != nil {
		return _err
	}
	file.Write(buffer.Bytes())
	return nil
}

func PackAndCompress(dir string, files []string, outputFile string, key []byte, encrypt bool) error {
	outdir, _ := filepath.Split(outputFile)
	_err := os.MkdirAll(outdir, 0777)
	if _err != nil {
		return _err
	}

	var tarFile bytes.Buffer

	tarWriter := tar.NewWriter(&tarFile)
	_err = Tar(dir, files, tarWriter, "")

	if _err != nil {
		return _err
	}

	tarWriter.Close()
	gzipedBuffer, _err := Gzip(&tarFile)

	if _err != nil {
		return _err
	}

	if encrypt {
		encryptedBuffer, _err := Encode(&gzipedBuffer, key)
		if _err != nil {
			return _err
		}
		return WriteToFile(outputFile + ".encrypted", encryptedBuffer)
	} else {
		return WriteToFile(outputFile, gzipedBuffer)
	}
}

func handleError(e error) {
	if e != nil {
		panic(e)
	}
}

func UploadToS3(k s3gof3r.Keys, bucketName string, pathToFile string) error {
	s3 := s3gof3r.New("", k)
	b := s3.Bucket(bucketName)

	file, err := os.Open(pathToFile)
	if err != nil {
		return err
	}

	stats, _ := file.Stat()

	w, err := b.PutWriter(stats.Name(), nil, nil)
	if err != nil {
		return err
	}

	if _, err = io.Copy(w, file); err != nil { // Copy into S3
		return err
	}

	if err = w.Close(); err != nil {
		return err
	}

	return nil
}

func DownloadFromS3(k s3gof3r.Keys, bucketName, filename, outputDirPath string) error {
	s3 := s3gof3r.New("", k)
	b := s3.Bucket(bucketName)

	r, _, err := b.GetReader(filename, nil)
	if err != nil {
		return err
	}

	outputFile, err := os.Create(filepath.Join(outputDirPath, filename))
	if err != nil {
		return err
	}

	if _, err = io.Copy(outputFile, r); err != nil {
		return err
	}

	err = r.Close()
	if err != nil {
		return err
	}

	return nil
}

func SSHUploader(port int, id_rsa, host, user, srcFile, dstFile string) *scp.SecureCopier {
	return scp.NewSecureCopier(port, false, false, false, true, false, false, id_rsa, "", "", srcFile, host, user, dstFile)
}

func SSHDownloader(port int, id_rsa, host, user, srcFile, dstFile string) *scp.SecureCopier {
	return scp.NewSecureCopier(port, false, false, false, true, false, false, id_rsa, host, user, srcFile, "", "", dstFile)
}

func SSHExec(ssh *scp.SecureCopier) error {
	var o, e bytes.Buffer
	err, _ := ssh.Exec(nil, &o, &e)
	return err
}
/*
func ErrString(err error) (s *string) {
	if err != nil {
		tmp := err.Error()
		s = &tmp
	}
	return
}
*/
