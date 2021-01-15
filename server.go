package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"flag"
	"html/template"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"io/ioutil"
	"log"
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

type jsonResponse struct {
	Boxes []int    `json:"boxes"`
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
	files, _ := ioutil.ReadDir(path)
	for i, file := range files {
		file, err := os.Open(path + file.Name())
		if err != nil {
			log.Fatal(err)
		}
		response, err := http.Post("http://localhost:5000", "file", file)
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
		panels[i] = Panel{OriginalFilePath: path + file.Name(), RedactedImageBase64: "example", DetectedText: detectedText}
	}

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

	encodedImage := base64.StdEncoding.EncodeToString(content)

	return encodedImage
}

// The img tag in html requires us to specify what type of image is being passed in (ie png, jpeg)
// Ideally, this can be standarded by the redact function so this function is unnecessary
func getImageType(imgPath string) string {
	img, err := os.Open(imgPath)
	if err != nil {
		log.Fatal(err)
	}

	_, imgType, err := image.Decode(img)
	if err != nil {
		log.Fatal(err)
	}

	return imgType
}

// Redacts image passed in
// Boxes looks like this {"boxes":[[363,15,505,46],[14,15,178,62],[358,462,519,496],[3,417,109,495]]}
func redactImage(imgPath string, boxes [][]int) {
	originalImage, err := os.Open("levine_joshua_2.jpg")
	if err != nil {
		log.Fatal(err)
	}

	rectangle := image.NewRGBA(image.Rect(363, 15, 505, 46))
	red := color.RGBA{255, 0, 0, 255}

	draw.Draw(rectangle, rectangle.Bounds(), &image.Uniform{red}, image.ZP, draw.Src)
}
