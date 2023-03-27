package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	cp "github.com/otiai10/copy"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

const BACKUPPREFIX = "_backup"

func main() {
	var fullPathlocal string
	//Regular expression to check if the path has only capital letters or numbers
	regexpsavepath, err := regexp.Compile("^[A-Z0-9]+$")
	if err != nil {
		log.Fatal(err)
	}
	if runtime.GOOS == strings.ToLower("Windows") {
		fullPathlocal = backupLocalFilesWin(regexpsavepath)
	}
	//TODO : Other OS(s)
	fullpathRemote, sftpClient, sshSession := backupRemoteFiles(regexpsavepath)
	defer sftpClient.Close()
	defer sshSession.Close()
	syncFolder(fullpathRemote, fullPathlocal, sftpClient)
}

func backupLocalFilesWin(regexpsavepath *regexp.Regexp) string {
	color.Green("CREATING LOCAL BACKUP")
	savepathyuzubase := filepath.Join(os.Getenv("APPDATA"), "yuzu", "nand", "user", "save", "0000000000000000")
	files, err := os.ReadDir(savepathyuzubase)
	if err != nil {
		log.Fatal(err)
	}
	var fullpath string
	for _, file := range files {
		if file.IsDir() {
			pathSave := file.Name()
			isCorrectName := regexpsavepath.Match([]byte(pathSave))
			if isCorrectName {
				fullpath = filepath.Join(savepathyuzubase, file.Name())
				err := cp.Copy(fullpath, fullpath+BACKUPPREFIX)
				if err != nil {
					log.Fatal(err)
				}
				break
			}
		}
	}
	color.Green("DONE")
	return fullpath
}

func backupRemoteFiles(regexpsavepath *regexp.Regexp) (string, *sftp.Client, *ssh.Session) {
	color.Green("CREATING STEAMDECK BACKUP")
	savepathyuzudeckbase := "/run/media/mmcblk0p1/Emulation/storage/yuzu/nand/user/save/0000000000000000"

	sshConfig := &ssh.ClientConfig{
		User: "deck",
		Auth: []ssh.AuthMethod{
			ssh.Password(promptpasswd()),
			//ssh.Password("bananacar1"),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	sshClient, err := ssh.Dial("tcp", "192.168.1.157:22", sshConfig)
	if err != nil {
		log.Fatal(err)
	}

	sshSession, err := sshClient.NewSession()
	if err != nil {
		panic(err)
	}

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		panic(err)
	}
	var fullpath string
	fileinfo, _ := sftpClient.ReadDir(savepathyuzudeckbase)
	for i := 0; i < len(fileinfo); i++ {
		if fileinfo[i].IsDir() {
			pathSave := fileinfo[i].Name()
			isCorrectName := regexpsavepath.Match([]byte(pathSave))
			if isCorrectName {
				fullpath = filepath.Join(savepathyuzudeckbase, fileinfo[i].Name())
				//COPY RECURSIVELY IN REMOTE
				command := fmt.Sprintf("cp -Rf %s %s", fullpath, fullpath+BACKUPPREFIX)
				if runtime.GOOS == strings.ToLower("Windows") {
					command = strings.ReplaceAll(command, "\\", "/")
					fullpath = strings.ReplaceAll(fullpath, "\\", "/")
				}
				sshSession.Run(command)
				break
			}
		}
	}
	color.Green("DONE")
	return fullpath, sftpClient, sshSession
}

func promptpasswd() string {
	fmt.Print("Enter password: ")
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		panic(err)
	}
	return string(bytePassword)
}

func syncFolder(remotePath string, localPath string, sftpClient *sftp.Client) error {
	walker := sftpClient.Walk(remotePath)
	//Sync remote
	for walker.Step() {
		if walker.Err() != nil {
			log.Fatal(walker.Err())
			return walker.Err()
		}

		remoteFileInfo := walker.Stat()
		remoteFilePath := walker.Path()
		localFilePath := filepath.Join(localPath, strings.TrimPrefix(remoteFilePath, remotePath))
		color.Blue("Checking " + remoteFileInfo.Name())
		if remoteFileInfo.IsDir() {
			err := os.MkdirAll(localFilePath, remoteFileInfo.Mode())
			if err != nil {
				log.Fatal(err)
				return err
			}
			continue
		}

		localFileInfo, err := os.Stat(localFilePath)
		if err != nil && !os.IsNotExist(err) {
			log.Fatal(err)
			return err
		}

		if os.IsNotExist(err) {
			err = downloadFile(remoteFilePath, localFilePath, sftpClient)
			if err != nil {
				log.Fatal(err)
				return err
			}
			continue
		}

		if remoteFileInfo.ModTime().After(localFileInfo.ModTime()) {
			color.Red("File " + remoteFileInfo.Name() + " in steam deck is newer, downloading to local")
			err = downloadFile(remoteFilePath, localFilePath, sftpClient)
			if err != nil {
				log.Fatal(err)
				return err
			}
		} else if remoteFileInfo.ModTime().Before(localFileInfo.ModTime()) {
			color.Red("File " + remoteFileInfo.Name() + " in steam deck is older, uploading to deck")
			err = uploadFile(localFilePath, remoteFilePath, sftpClient)
			if err != nil {
				log.Fatal(err)
				return err
			}
		}
	}
	color.Green("FILE SYNC DONE!")
	return nil
}

func uploadFile(localPath string, remotePath string, sftpClient *sftp.Client) error {
	localFile, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer localFile.Close()

	remoteFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return err
	}
	defer remoteFile.Close()

	_, err = io.Copy(remoteFile, localFile)
	if err != nil {
		return err
	}

	localFileInfo, err := os.Stat(localPath)
	if err != nil {
		return err
	}

	return sftpClient.Chmod(remotePath, localFileInfo.Mode())
}

func downloadFile(remotePath string, localPath string, sftpClient *sftp.Client) error {
	remoteFile, err := sftpClient.Open(remotePath)
	if err != nil {
		return err
	}
	defer remoteFile.Close()

	localFile, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer localFile.Close()

	_, err = io.Copy(localFile, remoteFile)
	if err != nil {
		return err
	}

	remoteFileInfo, err := sftpClient.Stat(remotePath)
	if err != nil {
		return err
	}

	return os.Chtimes(localPath, time.Now(), remoteFileInfo.ModTime())
}
