package main

import (
	"bufio"
	"bytes"
	"embed"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

// Data is a wrapper for the panel array to be passed into our html template
type Data struct {
	Panels []Panel
}

// Panel is a data type representing a single "panel" or "row" in the html
type Panel struct {
	OriginalImageBase64 template.URL
	RedactedImageBase64 template.URL
	DetectedText        string
}

type redactorJSONResponse struct {
	Boxes [][]int  `json:"boxes"`
	Text  []string `json:"text"`
}

var panels []Panel

func main() {
	listenPort := flag.String("l", "8080", "Port to listen to")
	fileDirectory := flag.String("d", ".", "Directory containing the images to be processed")
	POSTRequestAddress := flag.String("a", "http://localhost:5000", "Address to send the POST request for python-redactor to; must include the beginning http://")
	noredact := flag.Bool("noredact", false, "If present, python-redactor will not be used and the webapp will simply display the images unredacted")
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
	processFiles(files, *noredact, path, *POSTRequestAddress)
	fmt.Println("Successfully initialized")
	http.HandleFunc("/", indexHandler)
	http.ListenAndServe(":"+*listenPort, nil)
}

func processFiles(files []os.FileInfo, noredact bool, path, POSTRequestAddress string) {
	client := &http.Client{}
	validExtensions := map[string]bool{"jpeg": true, "png": true}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		extension := filepath.Ext(file.Name())[1:]
		// if extension is jpg, it must be renamed to jpeg for the base64 image encoding to decode properly
		if extension == "jpg" {
			extension = "jpeg"
		}
		if !validExtensions[extension] {
			continue
		}
		filePath := path + "/" + file.Name()
		if !noredact {
			POSTRequest := preparePOSTRequest(filePath, POSTRequestAddress)
			response, err := client.Do(&POSTRequest)
			if err != nil {
				log.Fatal(err)
			}
			jsonResp, detectedText, err := unwrapRedactorResponse(*response)
			if err != nil {
				log.Println(err)
				continue
			}
			panels = append(panels, Panel{OriginalImageBase64: encodeImage(filePath, extension), RedactedImageBase64: redactImage(filePath, jsonResp.Boxes), DetectedText: detectedText})
		} else {
			panels = append(panels, Panel{OriginalImageBase64: encodeImage(filePath, extension)})
		}
	}
}

func unwrapRedactorResponse(response http.Response) (redactorJSONResponse, string, error) {
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Fatal(err)
	}
	var jsonResp = new(redactorJSONResponse)
	err = json.Unmarshal(body, &jsonResp)
	if err != nil {
		return redactorJSONResponse{}, "", err
	}
	detectedText := organizeDetectedText(jsonResp.Text)
	return *jsonResp, detectedText, nil
}

func organizeDetectedText(text []string) string {
	detectedText := ""
	for _, str := range text {
		detectedText = detectedText + str + "\n"
	}
	return detectedText
}

// Source : https://stackoverflow.com/questions/20205796/post-data-using-the-content-type-multipart-form-data
func preparePOSTRequest(filePath, POSTRequestAddress string) http.Request {
	file, err := os.Open(filePath)
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
	req, err := http.NewRequest("POST", POSTRequestAddress, &b)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	return *req
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	//go:embed preview.html.tmpl
	var content embed.FS
	t, err := template.ParseFS(content, "preview.html.tmpl")
	if err != nil {
		log.Fatal(err)
	}
	err = t.Execute(w, Data{Panels: panels})
	if err != nil {
		log.Fatal(err)
	}
}

// Needs to be in the form of "data:image/" + getImageType(fn) + ";base64," + encodeImage(fn)
func encodeImage(imgPath, extension string) template.URL {
	img, err := os.Open(imgPath)
	if err != nil {
		log.Fatal(err)
	}

	reader := bufio.NewReader(img)
	content, _ := ioutil.ReadAll(reader)

	encodedImage := base64.StdEncoding.EncodeToString(content)

	return template.URL("data:image/" + extension + ";base64," + encodedImage)
}

func redactImage(imgPath string, boxes [][]int) template.URL {
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
		line1 := image.Rect(boxes[i][0], boxes[i][1], boxes[i][2], boxes[i][1]+2)
		line2 := image.Rect(boxes[i][2], boxes[i][1], boxes[i][2]+2, boxes[i][3]+2)
		line3 := image.Rect(boxes[i][2], boxes[i][3], boxes[i][0], boxes[i][3]+2)
		line4 := image.Rect(boxes[i][0], boxes[i][3], boxes[i][0]+2, boxes[i][1])
		draw.Draw(convertedOriginal, line1, &image.Uniform{red}, image.Point{boxes[i][0], boxes[i][1]}, draw.Src)
		draw.Draw(convertedOriginal, line2, &image.Uniform{red}, image.Point{boxes[i][0], boxes[i][1]}, draw.Src)
		draw.Draw(convertedOriginal, line3, &image.Uniform{red}, image.Point{boxes[i][0], boxes[i][1]}, draw.Src)
		draw.Draw(convertedOriginal, line4, &image.Uniform{red}, image.Point{boxes[i][0], boxes[i][1]}, draw.Src)
	}

	var buff bytes.Buffer

	png.Encode(&buff, convertedOriginal)

	encodedString := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buff.Bytes())

	return template.URL(encodedString)
}
