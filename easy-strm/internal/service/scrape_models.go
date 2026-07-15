package service

import "encoding/xml"

type nfoUniqueID struct {
	Type    string `xml:"type,attr,omitempty"`
	Default string `xml:"default,attr,omitempty"`
	Value   string `xml:",chardata"`
}

type nfoMovie struct {
	XMLName       xml.Name     `xml:"movie"`
	Title         string       `xml:"title"`
	OriginalTitle string       `xml:"originaltitle,omitempty"`
	SortTitle     string       `xml:"sorttitle,omitempty"`
	Year          string       `xml:"year,omitempty"`
	Premiered     string       `xml:"premiered,omitempty"`
	Tagline       string       `xml:"tagline,omitempty"`
	Plot          string       `xml:"plot,omitempty"`
	Runtime       string       `xml:"runtime,omitempty"`
	Thumbs        []nfoThumb   `xml:"thumb,omitempty"`
	Fanart        *nfoFanart   `xml:"fanart,omitempty"`
	Genres        []string     `xml:"genre,omitempty"`
	Countries     []string     `xml:"country,omitempty"`
	Languages     []string     `xml:"language,omitempty"`
	Studios       []string     `xml:"studio,omitempty"`
	Writers       []string     `xml:"writer,omitempty"`
	Directors     []string     `xml:"director,omitempty"`
	Set           string       `xml:"set,omitempty"`
	UniqueID      *nfoUniqueID `xml:"uniqueid,omitempty"`
	TmdbID        string       `xml:"tmdbid,omitempty"`
	Rating        string       `xml:"rating,omitempty"`
	Actors        []nfoActor   `xml:"actor,omitempty"`
}

type nfoEpisode struct {
	XMLName   xml.Name     `xml:"episodedetails"`
	Title     string       `xml:"title"`
	ShowTitle string       `xml:"showtitle,omitempty"`
	Season    string       `xml:"season"`
	Episode   string       `xml:"episode"`
	Aired     string       `xml:"aired,omitempty"`
	Plot      string       `xml:"plot,omitempty"`
	Thumbs    []nfoThumb   `xml:"thumb,omitempty"`
	Directors []string     `xml:"director,omitempty"`
	Writers   []string     `xml:"writer,omitempty"`
	UniqueID  *nfoUniqueID `xml:"uniqueid,omitempty"`
	TmdbID    string       `xml:"tmdbid,omitempty"`
	Rating    string       `xml:"rating,omitempty"`
	Actors    []nfoActor   `xml:"actor,omitempty"`
}

type nfoThumb struct {
	Aspect  string `xml:"aspect,attr,omitempty"`
	Preview string `xml:"preview,attr,omitempty"`
	Value   string `xml:",chardata"`
}

type nfoFanart struct {
	Thumbs []nfoFanartThumb `xml:"thumb"`
}

type nfoFanartThumb struct {
	Preview string `xml:"preview,attr,omitempty"`
	Value   string `xml:",chardata"`
}

type nfoActor struct {
	Name  string `xml:"name"`
	Role  string `xml:"role,omitempty"`
	Thumb string `xml:"thumb,omitempty"`
}

type tmdbMovieDetail struct {
	ID                  int             `json:"id"`
	Title               string          `json:"title"`
	OriginalTitle       string          `json:"original_title"`
	Overview            string          `json:"overview"`
	Tagline             string          `json:"tagline"`
	ReleaseDate         string          `json:"release_date"`
	VoteAverage         float64         `json:"vote_average"`
	PosterPath          string          `json:"poster_path"`
	BackdropPath        string          `json:"backdrop_path"`
	Runtime             int             `json:"runtime"`
	Genres              []tmdbGenre     `json:"genres"`
	ProductionCountries []tmdbCountry   `json:"production_countries"`
	SpokenLanguages     []tmdbLanguage  `json:"spoken_languages"`
	ProductionCompanies []tmdbCompany   `json:"production_companies"`
	BelongsToCollection *tmdbCollection `json:"belongs_to_collection"`
	Credits             *tmdbCredits    `json:"credits"`
}

type tmdbTVDetail struct {
	ID              int            `json:"id"`
	Name            string         `json:"name"`
	OriginalName    string         `json:"original_name"`
	Overview        string         `json:"overview"`
	Tagline         string         `json:"tagline"`
	FirstAirDate    string         `json:"first_air_date"`
	VoteAverage     float64        `json:"vote_average"`
	PosterPath      string         `json:"poster_path"`
	BackdropPath    string         `json:"backdrop_path"`
	NumberOfSeasons int            `json:"number_of_seasons"`
	Genres          []tmdbGenre    `json:"genres"`
	OriginCountry   []string       `json:"origin_country"`
	SpokenLanguages []tmdbLanguage `json:"spoken_languages"`
	Networks        []tmdbNetwork  `json:"networks"`
	Credits         *tmdbCredits   `json:"credits"`
}

type tmdbEpisodeDetail struct {
	ID            int          `json:"id"`
	Name          string       `json:"name"`
	Overview      string       `json:"overview"`
	AirDate       string       `json:"air_date"`
	SeasonNumber  int          `json:"season_number"`
	EpisodeNumber int          `json:"episode_number"`
	VoteAverage   float64      `json:"vote_average"`
	StillPath     string       `json:"still_path"`
	Credits       *tmdbCredits `json:"credits"`
}

type tmdbGenre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type tmdbCountry struct {
	Name string `json:"name"`
}

type tmdbLanguage struct {
	Name string `json:"name"`
}

type tmdbCompany struct {
	Name string `json:"name"`
}

type tmdbCollection struct {
	Name string `json:"name"`
}

type tmdbNetwork struct {
	Name string `json:"name"`
}

type tmdbCredits struct {
	Cast []tmdbCast `json:"cast"`
	Crew []tmdbCrew `json:"crew"`
}

type tmdbCast struct {
	Name        string `json:"name"`
	Character   string `json:"character"`
	Order       int    `json:"order"`
	ProfilePath string `json:"profile_path"`
}

type tmdbCrew struct {
	Name        string `json:"name"`
	Job         string `json:"job"`
	Department  string `json:"department"`
	ProfilePath string `json:"profile_path"`
}
