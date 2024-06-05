package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

var debugMode = false

const (
	directorySize = 4096
)

func debug(format string, args ...interface{}) {
	if debugMode {
		fmt.Printf(format, args...)
	}
}

func main() {
	args := ParseArgs()

	transfer := NewTransfer(args)
	transfer.Start()
}

type Transfer struct {
	sync.Mutex

	src            string
	dst            string
	checkDuration  int
	ignoreExisting bool
	count          int

	table FileStatusTable
}

func NewTransfer(args Args) *Transfer {
	return &Transfer{
		src:            args.src,
		dst:            args.dst,
		checkDuration:  args.checkDuration,
		ignoreExisting: args.ignoreExisting,
		count:          args.count,
		table:          make(FileStatusTable),
	}
}

func (t *Transfer) Start() {
	t.InitExisting()
	if !t.ignoreExisting {
		t.Transfer(Task{src: t.src, dst: t.dst})
	}

	taskChan := make(chan Task)

	go func() {
		for {
			task := <-taskChan
			t.Transfer(task)
		}
	}()

	for {
		t.UpdateFileStatus()
		for target := range t.ScanTargets() {
			taskChan <- Task{
				src: target.path,
				dst: filepath.Join(t.dst, target.path),
			}
		}

		time.Sleep(time.Duration(t.checkDuration) * time.Second)
	}
}

func (t *Transfer) InitExisting() {
	t.Lock()
	defer t.Unlock()

	err := filepath.Walk(
		t.src,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			var fileType FileType
			if info.IsDir() {
				fileType = Directory
			} else {
				fileType = File
			}

			t.table.Add(path, fileType, int(info.Size()))
			t.table.SetStatus(path, Complete)

			return nil
		},
	)
	if err != nil {
		if os.IsNotExist(err) {
			debug("error: %s does not exist\n", t.src)
			os.Exit(1)
		} else {
			debug("error: %v\n", err)
		}
	}
}

func (t *Transfer) UpdateFileStatus() {
	err := filepath.Walk(
		t.src,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if path == t.src {
				return nil
			}

			t.Lock()
			defer t.Unlock()

			if info.IsDir() {
				if !t.table.IsExists(path) {
					debug("Found new directory: %s\n", path)
					t.table.AddNewDirectory(path)
				}
			} else {
				fileSize := int(info.Size())
				if !t.table.IsExists(path) {
					debug("Found new file: %s\n", path)
					t.table.AddNewFile(path, fileSize)
				} else if t.table.GetSize(path) != fileSize {
					debug("Found updated file: %s\n", path)
					t.table.SetSize(path, fileSize)
					t.table.SetStatus(path, Idle)
					t.table.ResetCount(path)
				}
			}

			return nil
		},
	)
	if err != nil {
		if os.IsNotExist(err) {
			debug("error: %s does not exist\n", t.src)
			os.Exit(1)
		} else {
			debug("error: %v\n", err)
		}
	}
}

func (t *Transfer) ScanTargets() <-chan FileStatus {
	targetChan := make(chan FileStatus)

	t.Lock()
	table := t.table
	t.Unlock()

	go func() {
		for path, fs := range table {
			if fs.status != Idle {
				continue
			}

			t.Lock()
			if fs.fileType == Directory {
				t.table.SetStatus(path, Transferring)
				targetChan <- fs
			} else {
				count := t.table.GetCount(path)
				if count >= t.count {
					t.table.ResetCount(path)
					t.table.SetStatus(path, Transferring)
					targetChan <- fs
				} else {
					t.table.IncrementCount(path)
				}
			}
			t.Unlock()
		}

		close(targetChan)
	}()

	return targetChan
}

func (t *Transfer) Transfer(task Task) {
	debug("Transfer: %s -> %s\n", task.src, task.dst)

	cmd := exec.Command("scp", "-r", task.src, task.dst)
	if err := cmd.Run(); err != nil {
		debug("error: %v\n", err)
	}

	t.Lock()
	defer t.Unlock()

	if !t.table.IsIdle(task.src) {
		t.table.SetStatus(task.src, Complete)
	}
}

type Task struct {
	src string
	dst string
}

type FileTransferStatus string

const (
	Idle         FileTransferStatus = "idle"
	Transferring FileTransferStatus = "transferring"
	Complete     FileTransferStatus = "complete"
)

type FileType string

const (
	File      FileType = "file"
	Directory FileType = "directory"
)

type FileStatus struct {
	path       string
	status     FileTransferStatus
	fileType   FileType
	size       int
	checkCount int
}

type FileStatusTable map[string]FileStatus

func (t *FileStatusTable) Add(path string, fileType FileType, size int) {
	(*t)[path] = FileStatus{
		path:       path,
		status:     Idle,
		fileType:   fileType,
		size:       size,
		checkCount: 0,
	}
}

func (t *FileStatusTable) AddNewFile(path string, size int) {
	t.Add(path, File, size)
}

func (t *FileStatusTable) AddNewDirectory(root string) {
	t.Add(root, Directory, directorySize)

	err := filepath.Walk(
		root,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if path == root {
				return nil
			}

			if info.IsDir() {
				t.Add(path, Directory, directorySize)
			} else {
				t.Add(path, File, int(info.Size()))
			}
			t.SetStatus(path, Complete)

			return nil
		},
	)
	if err != nil {
		debug("error: %v\n", err)
	}
}

func (t *FileStatusTable) Remove(path string) {
	if !t.IsExists(path) {
		return
	}
	delete(*t, path)
}

func (t *FileStatusTable) Get(path string) FileStatus {
	return (*t)[path]
}

func (t *FileStatusTable) IsExists(path string) bool {
	_, ok := (*t)[path]
	return ok
}

func (t *FileStatusTable) SetStatus(path string, status FileTransferStatus) {
	fs := (*t)[path]
	(*t)[path] = FileStatus{
		path:       path,
		status:     status,
		fileType:   fs.fileType,
		size:       fs.size,
		checkCount: fs.checkCount,
	}
}

func (t *FileStatusTable) GetCount(path string) int {
	fs := (*t)[path]
	return fs.checkCount
}

func (t *FileStatusTable) SetCount(path string, checkCount int) {
	fs := (*t)[path]
	(*t)[path] = FileStatus{
		path:       path,
		status:     fs.status,
		fileType:   fs.fileType,
		size:       fs.size,
		checkCount: checkCount,
	}
}

func (t *FileStatusTable) ResetCount(path string) {
	t.SetCount(path, 0)
}

func (t *FileStatusTable) IncrementCount(path string) {
	fs := (*t)[path]
	(*t)[path] = FileStatus{
		path:       path,
		status:     fs.status,
		fileType:   fs.fileType,
		size:       fs.size,
		checkCount: fs.checkCount + 1,
	}
}

func (t *FileStatusTable) GetSize(path string) int {
	fs := (*t)[path]
	return fs.size
}

func (t *FileStatusTable) SetSize(path string, size int) {
	fs := (*t)[path]
	(*t)[path] = FileStatus{
		path:       path,
		status:     fs.status,
		fileType:   fs.fileType,
		size:       size,
		checkCount: fs.checkCount,
	}
}

func (t *FileStatusTable) IsIdle(path string) bool {
	fs := (*t)[path]
	return fs.status == Idle
}

type Args struct {
	src            string
	dst            string
	checkDuration  int
	ignoreExisting bool
	count          int
}

func ParseArgs() Args {
	var checkDuration int
	var ignoreExisting bool
	var count int
	var help bool
	var verbose bool

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <src> <dst>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}
	flag.IntVar(&checkDuration, "n", 1, "Check duration. This program checks the source directory every n seconds.")
	flag.BoolVar(&ignoreExisting, "ignore-existing", false, "Ignore existing files. This program does not transfer existing files if this flag is set.")
	flag.IntVar(&count, "count", 0, "Check count. This program transfers files after checking n times. If the file size is updated, the check count is reset. This option is useful for transferring large files that are updated frequently.")
	flag.BoolVar(&help, "h", false, "Show help.")
	flag.BoolVar(&verbose, "v", false, "Verbose mode. This program outputs debug messages if this flag is set.")
	flag.Parse()

	args := flag.Args()
	if help || len(args) < 2 {
		flag.Usage()
		os.Exit(1)
	}

	if verbose {
		debugMode = true
	}

	return Args{
		src:            args[0],
		dst:            args[1],
		checkDuration:  checkDuration,
		ignoreExisting: ignoreExisting,
		count:          count,
	}
}
