package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/doloopwhile/logrusltsv"
)

func rename(src, dst string) error {
	return os.Rename(src, dst)
}

func mv(src, dst string) error {
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
	for {
		ticker := time.NewTicker(t)
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
	for {
		ticker := time.NewTicker(rate)
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

// サイズが変化しなくなったら通知する
func monitoringFile(srcDir string, samplingRate time.Duration, targets <-chan []string, result chan<- string) {
	for {
		select {
		case names := <-targets:
			for _, name := range names {
				_, err := monitoring(samplingRate, fmt.Sprintf("%s/%s", srcDir, name))
				if err != nil {
					continue
				}
				result <- name
			}
		}
	}
}

func init() {
	logrus.SetFormatter(&logrusltsv.Formatter{})
}

func main() {
	var (
		src               string
		dst               string
		newFileCheckRate  int64
		fileSizeCheckRate int64
		logdir            string
		logger            = logrus.New()
	)

	flag.StringVar(&src, "src", "", "source directory")
	flag.StringVar(&dst, "dst", "", "destination directory")
	flag.Int64Var(&newFileCheckRate, "nc", 60, "new file check rate (sec)")
	flag.Int64Var(&fileSizeCheckRate, "fc", 60, "file size check rate (sec)")
	flag.StringVar(&logdir, "log", "/dev/stderr", "log target")
	flag.Parse()

	if l, err := os.Create(logdir); err == nil {
		logger.Out = l
	} else {
		logger.Out = os.Stderr
	}

	monitored := make(chan []string, 1024)
	noChangeFile := make(chan string, 1024)

	go getMonitored(src, time.Duration(newFileCheckRate)*time.Second, monitored)
	go monitoringFile(src, time.Duration(fileSizeCheckRate)*time.Second, monitored, noChangeFile)

	for {
		file := <-noChangeFile
		if err := qmv(src+"/"+file, dst+"/"+file); err != nil {
			logger.Errorln(err)
		} else {
			if err := os.Symlink(dst+"/"+file, src+"/"+file); err != nil {
				logger.Errorln(err)
			} else {
				logger.Infoln("moved complete: ", file)
			}
		}
	}
}
