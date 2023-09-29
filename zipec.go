package main

import (
	"archive/zip"
	"io/fs"
	"path/filepath"
	//"bytes"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	//"io/ioutil"
	"errors"
	"io"
	//"time"
	//rs "golangrs"
)

//note `Abs` is frequently used in program. While `Abs` calls `Clean` and `Clean` docs mentions "The returned path ends in a slash only if it represents a root directory" and "Finally, any occurrences of slash are replaced by Separator"

type MainCtx struct {
	ExitStatusCode int
}

//undone many println(err) are not printing line number

func hndx(ctx *MainCtx, zipfilenm string, desfilenm string) {
	//? should you delete partial extracted files if failure occurred?
	desfilenm, err := filepath.Abs(desfilenm)
	if err != nil {
		fmt.Println(err)
		ctx.ExitStatusCode = 1
		return
	}
	//todo os.MkdirAll(desfilenm, 0755)
	fileInfo, err := os.Lstat(desfilenm)
	if err != nil {
		fmt.Println("Lstat err.")
		fmt.Println(err)
		ctx.ExitStatusCode = 1
		return
	}
	if !fileInfo.IsDir() {
		fmt.Println("Destination not supported")
		ctx.ExitStatusCode = 1
		return
	}
	rcloser, err := zip.OpenReader(zipfilenm)
	if err != nil {
		fmt.Println("Failed to open zip")
		fmt.Println(err)
		ctx.ExitStatusCode = 1
		return
	}
	defer rcloser.Close()
	desWithPSep := desfilenm
	if !strings.HasSuffix(desfilenm, string(os.PathSeparator)) {
		desWithPSep += string(os.PathSeparator)
	}
	for _, fex := range rcloser.File {
		path, err := filepath.Abs(filepath.Join(desfilenm, fex.Name))
		if err != nil {
			fmt.Println(err)
			ctx.ExitStatusCode = 1
			return
		}
		// Check for ZipSlip (Directory traversal)
		if !strings.HasPrefix(path, desWithPSep) {
			fmt.Println(`ZipSlip detected`)
			ctx.ExitStatusCode = 1
			return
		}
		if fex.FileInfo().IsDir() {
			err = os.MkdirAll(path, fex.Mode())
			if err != nil {
				fmt.Println(err)
				ctx.ExitStatusCode = 1
				return
			}
		} else {
			func() {
				err = os.MkdirAll(filepath.Dir(path), fex.Mode())
				if err != nil {
					fmt.Println(err)
					ctx.ExitStatusCode = 1
					return
				}
				rc, err := fex.Open()
				if err != nil {
					fmt.Println("Failed to extract zip")
					fmt.Println(err)
					ctx.ExitStatusCode = 1
					return
				}
				defer rc.Close()
				newfile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fex.Mode()) //note here fails if file exists, should modify?
				if err != nil {
					fmt.Println(err)
					ctx.ExitStatusCode = 1
					return
				}
				defer func() {
					err := newfile.Close()
					if err != nil {
						panic(errors.New(`Closing err`))
					}
				}()
				_, err = io.Copy(newfile, rc)
				if err != nil {
					panic(errors.New(`Copy err`))
				}
			}()
		}

	}
}

func hndr(ctx *MainCtx, zipfilenm string, srcfilenm string) {
	//? should you delete partial file if failure occurred?
	ptrfile, err := os.Create(zipfilenm)
	if err != nil {
		fmt.Println("Failed to create file") //todo
		ctx.ExitStatusCode = 1
		return
	}
	defer ptrfile.Close()
	//zipFileInfo, err := ptrfile.Stat()
	w := zip.NewWriter(ptrfile)
	defer func() {
		err = w.Close()
		if err != nil {
			fmt.Println(err)
			ctx.ExitStatusCode = 1
		}
	}()
	//var fileInfo FileInfo
	fileInfo, err := os.Lstat(srcfilenm)
	if err != nil {
		fmt.Println("Lstat err.")
		fmt.Println(err)
		ctx.ExitStatusCode = 1
		return
	}
	if fileInfo.Mode().IsRegular() {
		bName := fileInfo.Name()
		if strings.Contains(bName, "\\") {
			fmt.Println(`Backslash appearing in filename causes this program to stop. Because golang docs of zip#Writer.Create mentions "and only forward slashes are allowed"`)
			ctx.ExitStatusCode = 1
			return
		}
		zfheader := &zip.FileHeader{
			Name:     bName,
			Method:   zip.Deflate,
			Modified: fileInfo.ModTime(),
		}
		zfheader.SetMode(fileInfo.Mode())
		addedFile, err := w.CreateHeader(zfheader)
		if err != nil {
			fmt.Println("Creation err")
			fmt.Println(err)
			ctx.ExitStatusCode = 1
			return
		}
		pFile, err := os.Open(srcfilenm)
		if err != nil {
			fmt.Println("Open err")
			fmt.Println(err)
			ctx.ExitStatusCode = 1
			return
		}
		defer pFile.Close()
		_, err = io.Copy(addedFile, pFile)
		if err != nil {
			fmt.Println("Copy err")
			fmt.Println(err)
			ctx.ExitStatusCode = 1
			return
		}
	} else if fileInfo.IsDir() {
		var abspath string
		abspath, err = filepath.Abs(srcfilenm)
		if err != nil {
			fmt.Println(err)
			ctx.ExitStatusCode = 1
			return
		}
		//note Abs calls Clean, while Clean docs mentions "The returned path ends in a slash only if it represents a root directory"
		if strings.HasSuffix(abspath, string(os.PathSeparator)) {
			fmt.Println("Not supporting root dir")
			ctx.ExitStatusCode = 1
			return
		}
		vName := filepath.VolumeName(abspath)
		trailingStr := abspath[len(vName):]
		if vName == abspath || trailingStr == `\` || trailingStr == `/` {
			fmt.Println("Not supporting root dir")
			ctx.ExitStatusCode = 1
			return
		}
		bName := filepath.Base(abspath)
		if "" == bName {
			fmt.Println("Unexpected error. Empty base name.")
			ctx.ExitStatusCode = 1
			return
		}
		absWithPSep := abspath + string(os.PathSeparator)
		zipfilenmAbs, err := filepath.Abs(zipfilenm)
		if err != nil {
			panic(err)
		}
		if strings.HasPrefix(zipfilenmAbs, absWithPSep) {
			panic(errors.New(`Destination file is under source.`))
		}
		prefixLen := len(abspath) - len(bName)
		//timeNow := time.Now()
		err = filepath.WalkDir(abspath, func(path string, dirent fs.DirEntry, errRelatedToPath error) error {
			//note if cmd1 is ".", then the first `path` is really ".", while other `path` strings do NOT start with "."
			//note it seems path does not contain trailing slash for folders
			if errRelatedToPath != nil {
				fmt.Println(errRelatedToPath)
				ctx.ExitStatusCode = 1
				return filepath.SkipAll
			}
			if strings.Contains(dirent.Name(), "\\") {
				fmt.Println(`Backslash appearing in filename causes this program to stop. Because golang docs of zip#Writer.Create mentions "and only forward slashes are allowed"`)
				ctx.ExitStatusCode = 1
				return filepath.SkipAll
			}
			drFileInfo, err := dirent.Info()
			if err != nil {
				fmt.Println("Info err: " + path)
				fmt.Println(err)
				ctx.ExitStatusCode = 1
				return filepath.SkipAll
			}
			relName := strings.ReplaceAll(path[prefixLen:], `\`, `/`)
			if dirent.IsDir() {
				zfheader := &zip.FileHeader{
					Name:     relName + `/`,
					Method:   zip.Deflate,
					Modified: drFileInfo.ModTime(),
				}
				zfheader.SetMode(drFileInfo.Mode())
				_, err := w.CreateHeader(zfheader)
				if err != nil {
					fmt.Println("Creation err: " + path)
					fmt.Println(err)
					ctx.ExitStatusCode = 1
					return filepath.SkipAll
				}
			} else {
				if !dirent.Type().IsRegular() {
					fmt.Println("Not regular file: " + path)
					ctx.ExitStatusCode = 1
					return filepath.SkipAll //?? when symlink or other strange things are found, skip just one file instead of aborting?
				}
				//if os.SameFile(zipFileInfo, drFileInfo) {
				//	fmt.Println("Path conflict")
				//	ctx.ExitStatusCode = 1
				//	return filepath.SkipAll
				//}
				zfheader := &zip.FileHeader{
					Name:     relName,
					Method:   zip.Deflate,
					Modified: drFileInfo.ModTime(),
				}
				zfheader.SetMode(drFileInfo.Mode())
				addedFile, err := w.CreateHeader(zfheader)
				if err != nil {
					fmt.Println("Creation err: " + path)
					fmt.Println(err)
					ctx.ExitStatusCode = 1
					return filepath.SkipAll
				}
				pFile, err := os.Open(path)
				if err != nil {
					fmt.Println("Open err: " + path)
					fmt.Println(err)
					ctx.ExitStatusCode = 1
					return filepath.SkipAll
				}
				defer pFile.Close()
				_, err = io.Copy(addedFile, pFile)
				if err != nil {
					fmt.Println("Copy err: " + path)
					fmt.Println(err)
					ctx.ExitStatusCode = 1
					return filepath.SkipAll
				}
			}
			return nil
		})
		if err != nil {
			fmt.Println(err)
			ctx.ExitStatusCode = 1
		}
	} else {
		fmt.Println("Not dir or regular file") //todo
		ctx.ExitStatusCode = 1
	}
}

func main() {
	ctx := &MainCtx{}
	defer func() {
		if r := recover(); r != nil {
			fmt.Println(r)
			fmt.Println("DEBUG STACK:\n" + string(debug.Stack())) //this is copy-pasted from rs.LogWithStackIfValueIsNotNil
			ctx.ExitStatusCode = 1
		}
		os.Exit(ctx.ExitStatusCode)
	}()
	cmd1 := os.Args[1] //todo check bounds first
	switch cmd1 {
	case "r":
		zipfilenm := os.Args[2]
		srcfilenm := os.Args[3]
		hndr(ctx, zipfilenm, srcfilenm)
	case "x":
		zipfilenm := os.Args[2]
		desfilenm := os.Args[3]
		hndx(ctx, zipfilenm, desfilenm)
	default:
		fmt.Println("Unexpected command") //todo more detailed msg
		ctx.ExitStatusCode = 1
		return
	}
	fmt.Println(`ALL DONE SUCCESSFULLY`)
}
