package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/doloopwhile/logrusltsv"
)

func isExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func rename(src, dst string) error {
	if isExists(dst) {
		return errors.New(fmt.Sprintf("cannot moved: \"%s\" And \"%s\" is existed.", src, dst))
	}
	return os.Rename(src, dst)
}

func mv(src, dst string) error {
	if isExists(dst) {
		return errors.New(fmt.Sprintf("cannot moved: \"%s\" And \"%s\" is existed.", src, dst))
	}
	fSrc, err := os.Open(src)
	if err != nil {
		return err
	}
	defer fSrc.Close()
	fDst, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer fDst.Close()

	_, err = io.Copy(fDst, fSrc)
	if err != nil {
		return err
	}
	return os.Remove(src)
}

func qmv(src, dst string) error {
	if err := rename(src, dst); err != nil {
		if err := mv(src, dst); err != nil {
			return err
		}
	}
	return nil
}

func getFile(srcpath string, mem map[string]bool) []string {
	var ret []string
	infos, err := ioutil.ReadDir(srcpath)
	if err != nil {
		return nil
	}

	for _, info := range infos {
		if info.Mode()&os.ModeSymlink != os.ModeSymlink && !info.IsDir() {
			if _, ok := mem[info.Name()]; !ok {
				mem[info.Name()] = true
				ret = append(ret, info.Name())
			}
		} else if _, ok := mem[info.Name()]; ok {
			delete(mem, info.Name())
		}
	}
	return ret
}

func getMonitored(srcpath string, t time.Duration, result chan<- []string) {
	mem := map[string]bool{}
	ticker := time.NewTicker(t)
	for {
		select {
		case <-ticker.C:
			fileNames := getFile(srcpath, mem)
			result <- fileNames
		}
	}
}

func monitoring(rate time.Duration, name string) (string, error) {
	bufInfo, err := os.Lstat(name)
	if err != nil {
		return "", err
	}
	ticker := time.NewTicker(rate)
	for {
		select {
		case <-ticker.C:
			nowInfo, err := os.Lstat(name)
			if err != nil {
				return "", err
			}
			if nowInfo.Size() == bufInfo.Size() {
				return name, nil
			}
		}
	}
}

type fileInfo struct {
	name string
	e    error
}

// サイズが変化しなくなったら通知する
func monitoringFile(srcDir string, samplingRate time.Duration, targets <-chan []string, result chan<- fileInfo) {
	for {
		select {
		case names := <-targets:
			for _, name := range names {
				_, err := monitoring(samplingRate, fmt.Sprintf("%s/%s", srcDir, name))
				if err != nil {
					result <- fileInfo{"", err}
				} else {
					result <- fileInfo{name, nil}
				}
			}
		}
	}
}

type Ltsv struct {
	Formatter       logrusltsv.Formatter
	TimestampFormat string
	FullTimestamp   bool
}

func (l Ltsv) Format(entry *logrus.Entry) ([]byte, error) {
	return l.Formatter.Format(entry)
}

func main() {
	logrus.SetFormatter(&Ltsv{TimestampFormat: "2006-01-02 15:04:05"})

	var (
		src               string
		dst               string
		newFileCheckRate  int64
		fileSizeCheckRate int64
		logdir            string
	)

	flag.StringVar(&src, "src", "", "source directory")
	flag.StringVar(&dst, "dst", "", "destination directory")
	flag.Int64Var(&newFileCheckRate, "nc", 60, "new file check rate (sec)")
	flag.Int64Var(&fileSizeCheckRate, "fc", 60, "file size check rate (sec)")
	flag.StringVar(&logdir, "log", "/dev/stderr", "log target")
	flag.Parse()

	if dst == "" || src == "" {
		log.Fatalln("illegal args")
	}

	dst, _ = filepath.Abs(dst)
	src, _ = filepath.Abs(src)
	logdir, _ = filepath.Abs(logdir)

	if l, err := os.OpenFile(logdir, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644); err == nil {
		logrus.SetOutput(l)
	} else {
		logrus.SetOutput(os.Stderr)
	}

	monitored := make(chan []string, 1024)
	noChangeFile := make(chan fileInfo, 1024)

	go getMonitored(src, time.Duration(newFileCheckRate)*time.Second, monitored)
	go monitoringFile(src, time.Duration(fileSizeCheckRate)*time.Second, monitored, noChangeFile)

	for {
		select {
		case fileInfo := <-noChangeFile:
			if fileInfo.e == nil {
				file := fileInfo.name
				if err := qmv(src+"/"+file, dst+"/"+file); err != nil {
					logrus.Errorln(err)
				} else {
					if err := os.Symlink(dst+"/"+file, src+"/"+file); err != nil {
						logrus.Errorln(err)
					} else {
						logrus.Infoln("moved complete: ", file)
					}
				}
			} else {
				logrus.Errorln(fileInfo.e)
			}
		}
	}
}
