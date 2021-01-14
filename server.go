package main

import (
	"flag"
	"io/ioutil"
	"net/http"
	"os"
)

type Data struct {
	Panels []Panel
}

type Panel struct {
	OriginalFilePath    string
	RedactedImageBase64 string
	DetectedText        string
}

var p []Panel

func main() {
	listenPort := flag.String("l", "8080", "Port to listen to")
	fileDirectory := flag.String("d", ".", "Directory containing the images to be processed")
	flag.Parse()
	var filePaths []string
	var path string
	if *fileDirectory == "." {
		path, _ = os.Getwd()
	} else {
		path = *fileDirectory
	}
	files, _ := ioutil.ReadDir(path)
	for _, file := range files {
		filePaths = append(filePaths, path+file.Name())
	}

	http.HandleFunc("/", indexHandler)
	http.ListenAndServe(":"+*listenPort, nil)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {

}
