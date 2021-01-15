package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
)

type Data struct {
	Panels []Panel
}

type Panel struct {
	OriginalImageBase64 template.URL
	RedactedImageBase64 string
	DetectedText        string
}

type jsonResponse struct {
	Boxes [][]int  `json:"boxes"`
	Text  []string `json:"text"`
}

var panels []Panel

func main() {
	listenPort := flag.String("l", "8080", "Port to listen to")
	fileDirectory := flag.String("d", ".", "Directory containing the images to be processed")
	flag.Parse()
	var path string
	if *fileDirectory == "." {
		path, _ = os.Getwd()
	} else {
		path = *fileDirectory
	}
	files, err := ioutil.ReadDir(path)
	if err != nil {
		log.Fatal(err)
	}
	client := &http.Client{}
	for _, file := range files {
		splitString := strings.Split(file.Name(), ".")
		extension := splitString[len(splitString)-1]
		if extension == "jpg" {
			extension = "jpeg"
		}
		file, err := os.Open(path + "/" + file.Name())
		if err != nil {
			log.Fatal(err)
		}
		values := map[string]io.Reader{
			"file": file,
		}
		var b bytes.Buffer
		w := multipart.NewWriter(&b)
		for key, r := range values {
			var fw io.Writer
			if x, ok := r.(io.Closer); ok {
				defer x.Close()
			}
			if x, ok := r.(*os.File); ok {
				if fw, err = w.CreateFormFile(key, x.Name()); err != nil {
					log.Fatal(err)
				}
			} else {
				if fw, err = w.CreateFormField(key); err != nil {
					log.Fatal(err)
				}
			}
			if _, err = io.Copy(fw, r); err != nil {
				log.Fatal(err)
			}

		}
		w.Close()
		req, err := http.NewRequest("POST", "http://localhost:5000", &b)
		if err != nil {
			log.Fatal(err)
		}
		req.Header.Set("Content-Type", w.FormDataContentType())
		response, err := client.Do(req)
		if err != nil {
			log.Fatal(err)
		}
		body, err := ioutil.ReadAll(response.Body)
		if err != nil {
			log.Fatal(err)
		}
		var jsonResp = new(jsonResponse)
		err = json.Unmarshal(body, &jsonResp)
		if err != nil {
			log.Fatal(err)
		}
		detectedText := ""
		for _, str := range jsonResp.Text {
			detectedText = detectedText + str + "\n"
		}
		panels = append(panels, Panel{OriginalImageBase64: template.URL("data:image/" + extension + ";base64," + encodeImage(file.Name())), RedactedImageBase64: "example", DetectedText: detectedText})
	}
	fmt.Println("Successfully initialized")
	http.HandleFunc("/", indexHandler)
	http.ListenAndServe(":"+*listenPort, nil)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFiles("preview.html")
	if err != nil {
		log.Fatal(err)
	}
	err = t.Execute(w, Data{Panels: panels})
	if err != nil {
		log.Fatal(err)
	}
}

// Currently using imgPath for testing, will change once the redact image function is done.
// Needs to be in the form of "data:image/" + getImageType(fn) + ";base64," + encodeImage(fn)
func encodeImage(imgPath string) string {
	img, err := os.Open(imgPath)
	if err != nil {
		log.Fatal(err)
	}

	reader := bufio.NewReader(img)
	content, _ := ioutil.ReadAll(reader)

	encoded := base64.StdEncoding.EncodeToString(content)

	return encoded
}
