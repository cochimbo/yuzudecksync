package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"

	cp "github.com/otiai10/copy"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

const BACKUPPREFIX = "_backup"

func main() {
	//Regular expression to check if the path has only capital letters or numbers
	regexpsavepath, err := regexp.Compile("^[A-Z0-9]+$")
	if err != nil {
		log.Fatal(err)
	}
	if runtime.GOOS == strings.ToLower("Windows") {
		backupLocalFilesWin(regexpsavepath)
	}
	//TODO : Other OS(s)
	backupRemoteFiles(regexpsavepath)
}

func backupLocalFilesWin(regexpsavepath *regexp.Regexp) string {
	fmt.Println("CREATING LOCAL BACKUP")
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
	fmt.Println("DONE")
	return fullpath
}

func backupRemoteFiles(regexpsavepath *regexp.Regexp) string {
	fmt.Println("CREATING STEAMDECK BACKUP")
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
	defer sshClient.Close()

	sshSession, err := sshClient.NewSession()
	if err != nil {
		panic(err)
	}
	defer sshSession.Close()

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		panic(err)
	}
	defer sftpClient.Close()
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
				}
				sshSession.Run(command)
				break
			}
		}
	}
	fmt.Println("DONE")
	return fullpath
}

func promptpasswd() string {
	fmt.Print("Enter password: ")
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		panic(err)
	}
	return string(bytePassword)
}

func syncLocalRemote() {

}
