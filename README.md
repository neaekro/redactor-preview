# redactor-preview

## Description
Meant to be used in conjunction with the [python-redactor](https://github.com/neaekro/python-redactor)

This will start a web app at localhost:port which previews the text detected in all images in a given directory in the following format

[Original Image] [Image with bounding boxes on detected text] [Detected text]

## Usage
1. Prepare a directory that contains the image files you wish to preview
2. Start python-redactor's flask server, either through docker or server.py
3. Navigate to the directory containing server.go
4. Run the command below (-noredact is if you want just want to see your original files with no boxed text; -boxredact is if you want redacted red boxes over text)\
By default,\
-d = . (working directory)\
-l = 8080\
-a = localhost:5000\
-noredact = false\
-boxredact = false
```
go run server.go -d <file path here> -l <port to listen to here> -a <Address to where python-redactor is hosted> <-noredact> 
```
5. The console should log "Successfully initialized." At this point, you may navigate to your browser and go to localhost:port to preview your files. If you chose to redact your images, then the page should update as the files are processed. 
