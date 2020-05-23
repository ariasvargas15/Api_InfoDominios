package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/buaazp/fasthttprouter"
	"github.com/keltia/ssllabs"
	_ "github.com/lib/pq"
	"github.com/likexian/whois-go"
	"github.com/valyala/fasthttp"
)
// Insert a row into the "tbl_employee" table.
/*	if _, err := db.Exec(
		`INSERT INTO tbl_employee (full_name, department, designation, created_at, updated_at)
		VALUES ('Irshad', 'IT', 'Product Manager', NOW(), NOW());`); err != nil {
		log.Fatal(err)
	}

// Select Statement.
	rows, err := db.Query("select employee_id, full_name FROM tbl_employee;")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var employeeID int64
		var fullName string
		if err := rows.Scan(&employeeId, &fullName); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Employee Id : %d \t Employee Name : %s\n", employeeId, fullName)
	}
*/



type InfoServers struct{
	Servers 			[]Server `json:"servers"`
	ServersChanged 		bool `json:"servers_changed"`
	SslGrade 			string `json:"ssl_grade"`
	PreviousSslGrade 	string `json:"previous_ssl_grade"`
	Logo 				string `json:"logo"`
	Title 				string `json:"title"`
	IsDown 				bool `json:"is_down"`
}

type Server struct{
	Address  string `json:"address"`
	SslGrade string `json:"ssl_grade"`
	Country  string `json:"country"`
	Owner    string `json:"owner"`

}

type Domain struct {
	Dom string `json:"domain"`
}

type Historial struct {
	Items []Domain `json:"items"`
}

var info InfoServers
var db *sql.DB
var host string
var previousSsl string

func main(){
	var err error
	db, err = sql.Open("postgres","postgresql://root@localhost:26257/infodominios?sslmode=disable")
	if err != nil {
		log.Fatal("error al conectar ala base de datos: ", err)
	}

	router := fasthttprouter.New()
	router.GET("/search/:domain", BuscarDominio)
	router.GET("/history", MostrarHistorial)
	//router.GET("/prueba/:name", Prueba)


	log.Fatal(fasthttp.ListenAndServe(":7000", router.Handler))
}

/*func Prueba(ctx *fasthttp.RequestCtx){
	var ok bool
	host, ok = ctx.UserValue("name").(string)

	var ep []ssllabs.Endpoint
	if ok {
		ep = BuscarEndpoints()
	}
	servers := CrearServers(ep)
	info.Servers = servers
	json.NewEncoder(ctx).Encode(info)

}*/

func MostrarHistorial(ctx *fasthttp.RequestCtx){
	rows, err := db.Query("select * FROM historial;")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	sitios := make([]Domain, 0)
	for rows.Next() {
		var sitio string
		if err := rows.Scan(&sitio); err != nil {
			log.Fatal(err)
		}
		sitios = append(sitios, Domain{Dom: sitio })
	}

	historial := Historial{}
	historial.Items = sitios
	fmt.Println(historial)
	json.NewEncoder(ctx).Encode(historial)
}

func BuscarDominio(ctx *fasthttp.RequestCtx){
	info = InfoServers{}
	var ok bool
	host, ok = ctx.UserValue("domain").(string)

	if ok {
		fmt.Println(host)
		GuardarConsulta(host)
		var body []byte
		status,bodyR,_ := fasthttp.Get(body, "http://www." + host)
		ValidarEstadoPagina(status)

		lastQ := BuscarRegistro()

		if lastQ != "" && !CompararFechas(lastQ){
			SetDominio()
		} else {
			BuscarHTML(string(bodyR), ctx)
			BuscarServers()
			if lastQ != "" {
				ValidarCambioServers()
				BuscarPreviousSSL()
			}
			GuardarRegistro()
		}

	}
	json.NewEncoder(ctx).Encode(info)
}

func GuardarConsulta(host string) {
	rows, err := db.Query("select * FROM historial WHERE host='" + host + "';")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		query := "INSERT INTO historial VALUES ('" + host + "');"
		_, err2 := db.Exec(query)
		if err2 != nil {
			log.Fatal(err2)
		}
	}
}


func BuscarPreviousSSL(){
	rows, err := db.Query("select previous_ssl FROM dominio WHERE dominio='" + host + "';")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&info.PreviousSslGrade); err != nil {
			log.Fatal(err)
		}
	}
}


func SetDominio() {
	rows, err := db.Query("select ssl_grade, previous_ssl, logo, title FROM dominio WHERE dominio='" + host + "';")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&info.SslGrade, &info.PreviousSslGrade,  &info.Logo, &info.Title); err != nil {
			log.Fatal(err)
		}
	}
	SetServers()
}

func SetServers(){
	info.Servers = make([]Server, 0)

	rows, err := db.Query("select server, ssl_grade, country, owner FROM server WHERE dominio='" + host + "';")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	var aux Server
	for rows.Next() {
		if err := rows.Scan(&aux.Address, &aux.SslGrade,  &aux.Country, &aux.Owner); err != nil {
			log.Fatal(err)
		}
		info.Servers = append(info.Servers, aux)
	}


}

func ValidarCambioServers() {
	rows, err := db.Query("select server FROM server WHERE dominio='" + host + "';")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	ips := make([]string, 0)
	for i:= 0; rows.Next(); i++ {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			log.Fatal(err)
		}
		ips = append(ips, ip)
	}
	changed := ValidarIP(ips)
	info.ServersChanged = changed
	if changed {
		BorrarAntiguosServers()
	}

}

func ValidarIP(ips []string) bool{
	if len(ips) != len(info.Servers) {
		return true
	}
	cont := 0
	for _, x := range ips {
		for _, y := range info.Servers {
			if x == y.Address {
				cont++
			}
		}
	}
	return cont != len(ips)
}

func BorrarAntiguosServers() {
	query := "DELETE FROM server WHERE dominio='" + host + "';"

	_, err := db.Exec(query)
	if err != nil {
		log.Fatal(err)
	}
}

func BuscarHTML(body string, ctx *fasthttp.RequestCtx)  {
	BuscarTitle(body)
	BuscarImagen()
}

func BuscarServers(){
	var ep []ssllabs.Endpoint
	ep = BuscarEndpoints()
	servers := CrearServers(ep)
	info.Servers = servers
	SearchLessGrade()
}


func BuscarRegistro() string{
	rows, err := db.Query("select last_query FROM dominio WHERE dominio='" + host + "';")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	lastQuery := ""
	for rows.Next() {
		if err := rows.Scan(&lastQuery); err != nil {
			log.Fatal(err)
		}
	}
	return lastQuery
}

func CompararFechas(lastQuery string) bool{
	fecha := FechaActual()
	is := false
	if lastQuery[0:9] != fecha[0:9] {
		is = true
	} else {
		horaLast, _ := strconv.Atoi(lastQuery[11:12])
		horaAct, _ := strconv.Atoi(fecha[11:12])
		if  (horaAct - horaLast) > 1 {
			is = true
		} else if (horaAct - horaLast) == 1{
			minLast, _ := strconv.Atoi(lastQuery[14:15])
			minAct, _ := strconv.Atoi(fecha[14:15])
			if (minAct + 60) - minLast >= 60 {
				is = true
			}
		}
	}
	return is
}


func GuardarRegistro()  {
	servers := GenerarArrayServers()
	fecha := FechaActual()
	query := "INSERT INTO dominio (dominio, server_changed, ssl_grade, previous_ssl, logo, title, last_query, servers) " +
		"VALUES ('" + host + "','" +  strconv.FormatBool(info.ServersChanged) + "','" + info.SslGrade + "','" + info.PreviousSslGrade +
		"','" + info.Logo + "','" + info.Title + "', TIMESTAMPTZ '" + fecha + "'," + servers + ");"

	 _, err := db.Exec(query)
	 if err != nil {
		log.Fatal(err)
	}
	for _, a := range info.Servers {
		query := "INSERT INTO server (server, ssl_grade, country, owner, dominio) " +
			"VALUES ('" + a.Address + "','" +  a.SslGrade + "','" + a.Country + "','" + a.Owner +
			"','" + host + "');"
		fmt.Println(query)
		_, err := db.Exec(query)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func GenerarArrayServers() string {
	res := "ARRAY["
	for i, serv := range info.Servers {
		res += "'" + serv.Address + "'"
		if i != (len(info.Servers)-1) {
			res += ","
		}
	}
	res += "]"
	return res
}

func FechaActual() string{
	t := time.Now()
	fecha := fmt.Sprintf("%d-%02d-%02d %02d:%02d:%02d",
		t.Year(), t.Month(), t.Day(),
		t.Hour(), t.Minute(), t.Second())

	return fecha
}

func BuscarEndpoints() []ssllabs.Endpoint{

	var report *ssllabs.Host

	for report == nil || report.Endpoints == nil || len(report.Endpoints) == 0 {
		c, _ := ssllabs.NewClient()
		opts := make(map[string]string)
		opts["fromCache"] = "on"
		report, _ = c.Analyze(host, false, opts)
		fmt.Println("paso")
		fmt.Println(report.Endpoints)
	}
	return report.Endpoints
}

func CrearServers(endpoints []ssllabs.Endpoint)  []Server{
	servers := make([]Server, 0)
	var ip, ssl, country, owner string
	for _, s := range endpoints{
		ip = s.IPAddress
		ssl = s.Grade

		result, err := whois.Whois(ip)
		if err == nil {
			country = BuscarPorId(result, "Country")
			own := [...]string{"OrgName", "Owner", "Org-Name"}
			for _, o := range own {
				owner = BuscarPorId(result, o)
				if owner != "not found"{
					break
				}
			}
		}
		servers = append(servers, Server{Address: ip, SslGrade:ssl, Country: country, Owner: owner})
	}
	return servers
}

func BuscarPorId(texto string, tag string) string{
	lineas := strings.Split(texto, "\n")
	for _, s := range lineas {
		if strings.Contains(s, tag) || strings.Contains(s, strings.ToLower(tag)) {
			aux := strings.Split(s, ": ")
			r := regexp.MustCompile("\\s+")
			res := r.ReplaceAllString(aux[1], " ")
			return res
		}
	}
	return "not found"
}

func ValidarEstadoPagina(status int) {
	info.IsDown = status != 200
}

func BuscarTitle(text string) {
	body := strings.Split(text, "</title>")
	title := strings.Split(body[0], "<title")
	final := strings.Split(title[1], ">")
	if len(final) > 1 {
		info.Title = final[1]
	} else {
		info.Title = final[0]
	}
}


func SearchLessGrade() {
	mapita := make(map[string]int)
	mapita["A+"] = 1
	mapita["A"] = 2
	mapita["B"] = 3
	mapita["C"] = 4
	mapita["D"] = 5
	mapita["E"] = 6
	mapita["F"] = 7

	lessSsl := 0
	res := "not found"
	for _, i := range info.Servers {
		if lessSsl < mapita[i.SslGrade] {
			lessSsl = mapita[i.SslGrade]
			res = i.SslGrade
		}
	}
	info.SslGrade = res
}

func BuscarImagen(){
	response, err := http.Get("http://favicongrabber.com/api/grab/" + host)
	if err != nil {
		fmt.Printf("The HTTP request failed with error %s\n", err)
	} else {
		data, _ := ioutil.ReadAll(response.Body)
		var f interface{}
		json.Unmarshal(data, &f)

		m := f.(map[string]interface{})
		x := m["icons"].([]interface{})
		if len(x)==0 {
			info.Logo = "not found"
		} else {
			t := x[0].(map[string]interface{})
			info.Logo = t["src"].(string)
		}

	}
}

//func KMP(cadena string, pattern string) int{
//	n := len(cadena)
//	m := len(pattern)
//	tab := PrefixFunction(pattern)
//	seen:=0
//	for i:=0; i<n; i++{
//		for seen > 0 && cadena[i] != pattern[seen] {
//			seen = tab[seen-1]
//		}
//		if cadena[i] == pattern[seen] {
//			seen++
//		}
//		if seen == m {
//			return i
//		}
//	}
//	return -1
//}
//
//func PrefixFunction(cad string) []int{
//	n := len(cad)
//	pf := make([]int, n)
//	pf[0] = 0
//	j := 0
//	for i:=0 ; i < n ; i++ {
//		for j==1 && cad[i] != cad[j] {
//			j = pf[j-1]
//		}
//		if cad[i] == cad[j]{
//			j++
//		}
//		pf[i] = j
//	}
//	return pf
//}
