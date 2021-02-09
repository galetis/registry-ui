package main

import (
	"context"
	"crypto/tls"
	"embed"
	"flag"
	"fmt"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/julienschmidt/httprouter"
	"html/template"
	"log"
	"net/http"
)

//go:embed assets/*
var assets embed.FS

//go:embed template/*
var templates embed.FS

func main() {

	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	port := flag.Int("port", 80, "Http listener port")
	registryUrl := flag.String("url", "localhost:5000", "Url for registry endpoint")
	user := flag.String("user", "", "Username for registry auth")
	pass := flag.String("pass", "", "Password for registry auth")

	flag.Parse()

	router := httprouter.New()

	auth := remote.WithAuth(authn.FromConfig(authn.AuthConfig{
		Username: *user,
		Password: *pass,
	}))

	registry, err := name.NewRegistry(*registryUrl, name.Insecure)
	if err != nil {
		log.Fatal(err)
	}

	tmplRead, _ := templates.ReadFile("template/index.html")

	tmpl, _ := template.New("tmpl").Parse(string(tmplRead))

	router.ServeFiles("/static/*filepath", http.FS(assets))

	router.GET("/", func(writer http.ResponseWriter, request *http.Request, params httprouter.Params) {

		repos, err := remote.Catalog(context.Background(), registry, auth)

		if err != nil {
			sendError(writer, err)
			return
		}

		repo := request.URL.Query().Get("repo")
		tag := request.URL.Query().Get("tag")

		data := map[string]interface{}{
			"Registry":     registry.String(),
			"Repositories": repos,

			"SelectedRepo": repo,
			"SelectedTag":  tag,
		}

		if repo != "" {
			repoName, _ := name.NewRepository(registry.Name() + "/" + repo)
			tags, err := remote.List(repoName, auth)
			if err != nil {
				sendError(writer, err)
				return
			}

			data["Tags"] = tags

			if tag != "" {

				tagName, _ := name.NewTag(repoName.String() + ":" + tag)
				img, err := remote.Image(tagName, auth)
				if err != nil {
					sendError(writer, err)
					return
				}

				digest, _ := img.Digest()
				layers, _ := img.Layers()

				var imgSize int64
				for _, layer := range layers {
					size, _ := layer.Size()
					imgSize += size
				}

				data["TagDetail"] = map[string]interface{}{
					"Name":   tag,
					"Digest": digest.String(),
					"Size":   ByteCountSI(imgSize),
				}

			}

		}

		if err := tmpl.Execute(writer, data); err != nil {
			sendError(writer, err)
		}

	})

	listen := fmt.Sprintf(":%d", *port)
	log.Print(fmt.Sprintf("Listening on %s", listen))
	log.Fatal(http.ListenAndServe(listen, router))

}

func ByteCountSI(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB",
		float64(b)/float64(div), "kMGTPE"[exp])
}

func sendError(w http.ResponseWriter, message error) {
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte(message.Error()))
}
