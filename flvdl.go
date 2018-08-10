package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	Url "net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"gopkg.in/yaml.v2"
	"github.com/palvarezcordoba/wget-go/wget"
	"os/exec"
)

// nameRegexp es una expresión regular para obtener el nombre de una página web.
var (
	nameRegexp = *regexp.MustCompile(`https?://(?:\w*\.)*(\w*)\.\w*/?`)
	reader     = bufio.NewReader(os.Stdin)
	_chapters  = flag.String("c", "", "Rango de capítulos")
)

// AnimeResult representa un anime resultante de una búsqueda.
type AnimeResult struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`
	Url         string `yaml:"url"`
	LastChapter string `yaml:"lastChapter"`
}

// gyetInt se usa para convertir un string a int.
func getInt(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return n
}

// getZippyshareDownloadLink recibe un link de zippyshare y devuelve
// el link de descarga directa.
func getZippyshareDownloadLink(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`"(/d/.*?/)" \+ \((\d+) % (\d+) \+ (\d+) % (\d+)\) \+ "([^"]*)"`)
	js := re.FindStringSubmatch(string(bytes))
	n := getInt(js[2])%getInt(js[3]) + getInt(js[4])%getInt(js[5])
	scheme := resp.Request.URL.Scheme
	host := resp.Request.URL.Host
	path := fmt.Sprintf("%s%d%s", js[1], n, js[6])
	if err != nil {
		return "", err
	}
	downloadLink := fmt.Sprintf("%s://%s%s", scheme, host, path)
	return downloadLink, nil
}

// getDoc descarga una página web, y crea un goquery.Document.
func getDoc(url string) *goquery.Document {
	resp, err := http.Get(url)
	if err != nil {
		panic(err)
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		panic(err)
	}
	return doc
}

// getDownloadLinks devuelve los links de descarga existentes para
// un anime.
func getDownloadLinks(url string) map[string]string {
	doc := getDoc(url)
	urls := make(map[string]string, 0)
	selection := doc.Find(`#DwsldCn > div > table > tbody > tr > td:nth-child(4) > a`)
	selection.Each(func(i int, selection *goquery.Selection) {
		val, exists := selection.Attr("href")
		if !exists {
			return
		}
		s, _ := Url.QueryUnescape(val)
		url := strings.Split(s, "=")[1]
		name := nameRegexp.FindStringSubmatch(url)[1]
		urls[name] = url
	})

	return urls
}

// searchAnime busca un anime en animeflv.net y devuelve un
// mapa con los resultados.
func searchAnime(anime string) (map[int]AnimeResult, []int) {
	anime = strings.Replace(anime, " ", "+", -1)
	url := "https://animeflv.net/browse?q=" + anime
	resp, err := http.Get(url)
	if err != nil {
		panic(err)
	}
	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	exp := regexp.MustCompile(`(?s)<li>.*?<article class="Anime alt B">.*?<a href="(.*?)">.*?<span class="Type.*?">(.*?)</span>.*?<h3 class="Title">(.*?)(?:\.\.\.)?</h3>`)
	matches := exp.FindAllStringSubmatch(string(bytes), -1)
	var animes = make(map[int]AnimeResult, 24)
	var keys []int
	for i := range matches {
		keys = append(keys, i)
		animes[i] = AnimeResult{
			Url:  "https://animeflv.net" + matches[i][1],
			Type: matches[i][2],
			Name: matches[i][3]}
	}
	sort.Ints(keys)
	// TODO: NO usar keys, usar un for y len(animes)
	return animes, keys
}

// downloadWithWget ejecuta wget sobre url.
func downloadWithWget(url string, cap string) error {
	args := make([]string, 1)
	args = append(args, url)
	inPipe := os.Stdin
	outPipe := os.Stdout
	wgetter := new(wget.Wgetter)
	wgetter.AlwaysPipeStdin = false
	err, _ := wgetter.ParseFlags(args, outPipe)
	if err != nil {
		return err
	}
	wgetter.OutputFilename = cap + ".mp4"
	wgetter.Timeout = 10
	wgetter.Retries = 5
	err, _ = wgetter.Exec(inPipe, outPipe, outPipe)
	//cmd := exec.Command("wget", url)
	//f, err := pty.Start(cmd)
	if err != nil {
		panic(err)
	}
	//go io.Copy(os.Stdout, f)
	return nil
}

// askAnime pregunta al usuario cuál anime quiere ver
// y devuelve la entrada.
func askAnime() string {
	fmt.Print("¿Qué ánime deseas ver?\n> ")
	anime, err := reader.ReadString('\n')
	anime = strings.TrimSpace(anime)
	if err != nil {
		panic(err)
	}
	return anime
}

// selectAnime pide al usuario que elija un anime entre
// varios, y devuelve el índice del elemento seleccionado.
func selectAnime(animes map[int]AnimeResult, keys []int) int {
	fmt.Print("Elige uno:\n\n")
	for _, v := range keys {
		fmt.Println(v, animes[v].Name)
	}
	fmt.Print("> ")
	x, err := reader.ReadString('\n')
	x = strings.TrimSpace(x)
	if err != nil {
		panic(err)
	}
	i := getInt(x)
	return i
}

// askChapter pregunta al usario cuál capítulo quiere ver.
func askChapter() string {
	fmt.Print("¿Qué capítulo quieres ver?\n> ")
	chapter, err := reader.ReadString('\n')
	chapter = strings.TrimSpace(chapter)
	if err != nil {
		panic(err)
	}
	return chapter
}

// saveAnime guarda en formato yaml un AnimeResult.
func saveAnime(anime AnimeResult) {
	w, err := os.OpenFile("anime.yaml", os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}
	yamlEncoder := yaml.NewEncoder(w)
	defer yamlEncoder.Close()
	defer w.Close()
	yamlEncoder.Encode(anime)
}

// loadAnime hacer Unmarshal de un AnimeResult
// a partir de un fichero yaml.
func loadAnime() *AnimeResult {
	r, err := os.Open("anime.yaml")
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		panic(err)
	}
	yamlDecoder := yaml.NewDecoder(r)
	anime := new(AnimeResult)
	yamlDecoder.Decode(anime)
	return anime
}

// getChapter ejecuta askChapter si len(anime.LastChapter) == 0, sino
// se preguntará si se desea ver el capítulo anime.LastChapter
// del anime anime.Name.
func getChapter(anime *AnimeResult) []string {
	var chapter string
	if len(anime.LastChapter) == 0 {
		chapter = askChapter()
	} else {
		fmt.Printf("¿Quieres reproducir el capítulo %d del anime %s? (s/n): ",
			getInt(anime.LastChapter)+1, anime.Name)
		want, err := reader.ReadString('\n')
		if err != nil {
			panic(err)
		}
		want = strings.TrimSpace(want)
		if want == "s" || want == "" {
			chapter = strconv.Itoa(getInt(anime.LastChapter) + 1)
		} else {
			chapter = askChapter()
		}
	}
	chapters := []string{chapter}
	return chapters
}

func getDownloadLinkWithYoutubedl(url string) string {
	out, err := exec.Command("youtube-dl", "-g", url).Output()
	if err != nil {
		return ""
	}
	s := string(out)
	s = strings.Replace(s, "\n", "", 1)
	matched, err := regexp.MatchString("^(http:\\/\\/www\\.|https:\\/\\/www\\.|http:\\/\\/|https:\\/\\/)?[a-z0-9]+([\\-\\.]{1}[a-z0-9]+)*\\.[a-z]{2,5}(:[0-9]{1,5})?(\\/.*)?$", s)
	if err != nil {
		return ""
	}
	if !matched {
		return ""
	}
	return s
}

func downloadChapter(anime *AnimeResult, chapter string) {
	_saveAnime := func() {
			anime.LastChapter = chapter
			saveAnime(*anime)
	}
	url := strings.Replace(anime.Url, "/anime/", "/ver/", 1)
	url += "-" + chapter
	links := getDownloadLinks(url)
	link, err := getZippyshareDownloadLink(links["zippyshare"])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, "Error al obtener el link de zippyshare.")
		// Uso goto porque lo hago de una forma no confusa, y para evitar
		// tener bloques if innecesarios.
		goto ForLoop
	}
	err = downloadWithWget(link, chapter)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, "Error al descargar desde zippyshare, intentando con otros proveedores...")
	} else {
		_saveAnime()
		return
	}

  ForLoop:
	delete(links, "zippyshare")
	for k, v := range links {
		link := getDownloadLinkWithYoutubedl(v)
		if link != "" {
			err = downloadWithWget(link, chapter)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				fmt.Fprintf(os.Stderr, "Error al descargar desde %s, intentando con otros proveedores...", k)
				continue
			}
			_saveAnime()
			return
		}
	}
}

func init() {
	flag.Parse()
}

func main() {
	anime := loadAnime()
	if anime == nil {
		inpt := askAnime()
		animes, keys := searchAnime(inpt)
		i := selectAnime(animes, keys)
		_anime := animes[i]
		anime = &_anime
	}
	var chapters []string
	if *_chapters == "" {
		chapters = getChapter(anime)
	} else {
		s := strings.Split(*_chapters, "-")
		if len(s) != 2 {
			panic("Rango de capítulos incorrecto.")
		}
		i := getInt(s[0])
		i2 := getInt(s[1])
		if i2 <= i {
			panic("Rango de capítulos incorrecto.")
		}
		for ; i <= i2; i++ {
			chapters = append(chapters, strconv.Itoa(i))
		}
	}
	for _, chapter := range chapters {
		downloadChapter(anime, chapter)
	}
}


