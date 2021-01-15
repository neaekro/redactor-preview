package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
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
		panels = append(panels, Panel{OriginalFilePath: file.Name(), RedactedImageBase64: "example", DetectedText: detectedText})
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

func redactImage(imgPath string, boxes [][]int) *image.RGBA {
	originalImage, err := os.Open(imgPath)
	if err != nil {
		log.Fatal(err)
	}

	original, _, err := image.Decode(originalImage)
	if err != nil {
		log.Fatal(err)
	}

	b := original.Bounds()
	convertedOriginal := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(convertedOriginal, convertedOriginal.Bounds(), original, b.Min, draw.Src)

	red := color.RGBA{255, 0, 0, 255}

	for i := 0; i < len(boxes); i++ {
		r := image.Rect(boxes[i][0], boxes[i][1], boxes[i][2], boxes[i][3])
		fmt.Println(r)
		line1 := image.Rect(boxes[i][0], boxes[i][1], boxes[i][2], boxes[i][1]+2)
		line2 := image.Rect(boxes[i][2], boxes[i][1], boxes[i][2]+2, boxes[i][3]+2)
		line3 := image.Rect(boxes[i][2], boxes[i][3], boxes[i][0], boxes[i][3]+2)
		line4 := image.Rect(boxes[i][0], boxes[i][3], boxes[i][0]+2, boxes[i][1])
		draw.Draw(convertedOriginal, line1, &image.Uniform{red}, image.Point{boxes[i][0], boxes[i][1]}, draw.Src)
		draw.Draw(convertedOriginal, line2, &image.Uniform{red}, image.Point{boxes[i][0], boxes[i][1]}, draw.Src)
		draw.Draw(convertedOriginal, line3, &image.Uniform{red}, image.Point{boxes[i][0], boxes[i][1]}, draw.Src)
		draw.Draw(convertedOriginal, line4, &image.Uniform{red}, image.Point{boxes[i][0], boxes[i][1]}, draw.Src)
	}

	return convertedOriginal

}
