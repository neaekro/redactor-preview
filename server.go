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
	"strconv"
)

// Data is a wrapper for the panel array to be passed into our html template
type Data struct {
	Panels []Panel
}

// Panel is a data type representing a single "panel" or "row" in the html
type Panel struct {
	OriginalImageBase64 template.URL
	FilePath            string
}

type redactorJSONResponse struct {
	Boxes [][]int  `json:"boxes"`
	Text  []string `json:"text"`
}

type JSONReturn struct {
	RedactedImageBase64 string
	DetectedText        string
}

var panels []Panel
var noredact *bool
var POSTRequestAddress *string
var boxredact *bool

//go:embed preview.html.tmpl
var preview embed.FS

//go:embed previewNoredact.html.tmpl
var previewNoredact embed.FS

func main() {
	listenPort := flag.String("l", "8080", "Port to listen to")
	fileDirectory := flag.String("d", ".", "Directory containing the images to be processed")
	POSTRequestAddress = flag.String("a", "http://localhost:5000", "Address to send the POST request for python-redactor to; must include the beginning http://")
	noredact = flag.Bool("noredact", false, "If present, python-redactor will not be used and the webapp will simply display the images unredacted")
	boxredact = flag.Bool("boxredact", false, "If true, detected text will be overriden with a red box")
	flag.Parse()
	var path string
	if *noredact && *boxredact {
		fmt.Println("Oops, cannot pass both -noredact and -boxredact together. You can't redact and not redact at the same time!")
		fmt.Println("Exiting...")
		return
	}
	if *fileDirectory == "." {
		path, _ = os.Getwd()
	} else {
		path = *fileDirectory
	}
	files, err := ioutil.ReadDir(path)
	if err != nil {
		log.Println("ReadDir: ", err)
		fmt.Println("Oops, there was an error in reading the directory provided. Maybe I don't have the right permissions?")
		fmt.Println("Exiting...")
		return
	}
	processFiles(files, path)
	fmt.Println("Successfully initialized")
	fmt.Println("Listening at http://localhost:" + *listenPort)
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/img/", imgHandler)
	if err := http.ListenAndServe(":"+*listenPort, nil); err != nil {
		log.Println("ListenAndServe: ", err)
		fmt.Println("There was an error hosting on the specified port")
		fmt.Println("Exiting...")
	}
}

func processFiles(files []os.FileInfo, path string) {
	validExtensions := map[string]bool{"jpeg": true, "png": true}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		extension := filepath.Ext(file.Name())
		if extension == "" {
			continue
		}
		extension = extension[1:]
		// if extension is jpg, it must be renamed to jpeg for the base64 image encoding to decode properly
		if extension == "jpg" {
			extension = "jpeg"
		}
		if !validExtensions[extension] {
			continue
		}
		filePath := path + "/" + file.Name()
		panels = append(panels, Panel{OriginalImageBase64: encodeImage(filePath, extension), FilePath: filePath})
	}
}

func unwrapRedactorResponse(response http.Response) (redactorJSONResponse, string, error) {
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return redactorJSONResponse{}, "", err
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
func preparePOSTRequest(file *os.File, POSTRequestAddress string) (http.Request, error) {
	values := map[string]io.Reader{
		"file": file,
	}
	var err error
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	for key, r := range values {
		var fw io.Writer
		if x, ok := r.(io.Closer); ok {
			defer x.Close()
		}
		if x, ok := r.(*os.File); ok {
			if fw, err = w.CreateFormFile(key, x.Name()); err != nil {
				return http.Request{}, err
			}
		} else {
			if fw, err = w.CreateFormField(key); err != nil {
				return http.Request{}, err
			}
		}
		if _, err = io.Copy(fw, r); err != nil {
			return http.Request{}, err
		}
	}
	w.Close()
	req, err := http.NewRequest("POST", POSTRequestAddress, &b)
	if err != nil {
		return http.Request{}, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	return *req, nil
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if !*noredact {
		t, err := template.ParseFS(preview, "preview.html.tmpl")
		if err != nil {
			log.Println("Error parsing html template: ", err)
			return
		}
		err = t.Execute(w, Data{Panels: panels})
		if err != nil {
			log.Println("Error executing html template: ", err)
			return
		}
	} else {
		t, err := template.ParseFS(previewNoredact, "previewNoredact.html.tmpl")
		if err != nil {
			log.Println("Error parsing html template: ", err)
			return
		}
		err = t.Execute(w, Data{Panels: panels})
		if err != nil {
			log.Println("Error executing html template: ", err)
			return
		}
	}
}

func imgHandler(w http.ResponseWriter, r *http.Request) {
	param := r.URL.Query()
	panelIndex, err := strconv.Atoi(param.Get("panelIndex"))
	if err != nil {
		log.Println("No panelIndex parameter passed? ", err)
		return
	}
	workingPanel := panels[panelIndex]
	file, err := os.Open(workingPanel.FilePath)
	if err != nil {
		log.Println("Error reading "+workingPanel.FilePath+": ", err)
		return
	}
	POSTRequest, err := preparePOSTRequest(file, *POSTRequestAddress)
	if err != nil {
		log.Println("Error creating POST request for file "+workingPanel.FilePath+": ", err)
		return
	}
	client := &http.Client{}
	response, err := client.Do(&POSTRequest)
	if err != nil {
		log.Println("Error sending POST request for "+workingPanel.FilePath+": ", err)
		fmt.Println("Perhaps you didn't run the python-redactor?")
		return
	}
	jsonResp, detectedText, err := unwrapRedactorResponse(*response)
	if err != nil {
		log.Println("Error unwrapping response for "+workingPanel.FilePath+": ", err)
		return
	}
	returnValue := JSONReturn{RedactedImageBase64: redactImage(workingPanel.FilePath, jsonResp.Boxes), DetectedText: detectedText}
	jsonData, err := json.Marshal(returnValue)
	if err != nil {
		log.Println("Error wrapping return into a JSON for "+workingPanel.FilePath+": ", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonData)
}

// Returns in the form of "data:image/" + getImageType(fn) + ";base64," + encodeImage(fn)
func encodeImage(imgPath, extension string) template.URL {
	img, err := os.Open(imgPath)
	if err != nil {
		fmt.Println("There was an error reading " + imgPath + "; replacing with a filler")
		file, err := os.ReadFile("404.txt")
		if err != nil {
			fmt.Println("Wow, looks like I can't even open my filler image :(")
			return template.URL("")
		}
		return template.URL(string(file))
	}

	reader := bufio.NewReader(img)
	content, _ := ioutil.ReadAll(reader)

	encodedImage := base64.StdEncoding.EncodeToString(content)

	return template.URL("data:image/" + extension + ";base64," + encodedImage)
}

func redactImage(imgPath string, boxes [][]int) string {
	originalImage, err := os.Open(imgPath)
	if err != nil {
		fmt.Println("There was an error reading " + imgPath + "; replacing with a filler")
		file, err := os.ReadFile("404.txt")
		if err != nil {
			fmt.Println("Wow, looks like I can't even open my filler image :(")
			return ""
		}
		return string(file)
	}

	original, _, err := image.Decode(originalImage)
	if err != nil {
		log.Println("Error decoding "+imgPath+": ", err)
		file, err := os.ReadFile("404.txt")
		if err != nil {
			fmt.Println("Wow, looks like I can't even open my filler image :(")
			return ""
		}
		return string(file)
	}

	b := original.Bounds()
	convertedOriginal := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(convertedOriginal, convertedOriginal.Bounds(), original, b.Min, draw.Src)

	red := color.RGBA{255, 0, 0, 255}

	if !*boxredact {
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
	} else {
		for i := 0; i < len(boxes); i++ {
			box := image.Rect(boxes[i][0], boxes[i][1], boxes[i][2], boxes[i][3])
			draw.Draw(convertedOriginal, box, &image.Uniform{red}, image.Point{boxes[i][0], boxes[i][1]}, draw.Src)
		}
	}

	var buff bytes.Buffer

	png.Encode(&buff, convertedOriginal)

	encodedString := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buff.Bytes())

	return encodedString
}
