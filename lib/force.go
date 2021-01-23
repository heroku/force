package lib

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/ForceCLI/force/lib/query"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ForceCLI/force/desktop"
	. "github.com/ForceCLI/force/error"
)

var (
	ClientId    = "3MVG9ytVT1SanXDnX_hOa9Ys5NxVp5C26JlyQjwr.xTJtUqoKonXY.M8CcjoEknMrV4YUvPvXLiMyzI.Aw23C"
	RedirectUri = "http://localhost:3835/oauth/callback"
)

type APIError struct {
	ErrorMessage string
}

func (m *APIError) Error() string {
	return m.ErrorMessage
}

var Timeout int64 = 0
var CustomEndpoint = ``
var SessionExpiredError = errors.New("Session expired")
var APILimitExceededError = &APIError{"API limit exceeded"}
var ClassNotFoundError = errors.New("class not found")
var MetricsNotFoundError = errors.New("metrics not found")
var DevHubOrgRequiredError = errors.New("Org must be a Dev Hub")

const (
	EndpointProduction = iota
	EndpointTest       = iota
	EndpointPrerelease = iota
	EndpointMobile1    = iota
	EndpointCustom     = iota
)

const (
	RefreshUnavailable = iota
	RefreshOauth       = iota
	RefreshSFDX        = iota
)

type RefreshMethod int

type Force struct {
	Credentials *ForceSession
	Metadata    *ForceMetadata
	Partner     *ForcePartner
}

type UserInfo struct {
	UserName     string `json:"preferred_username"`
	OrgId        string `json:"organization_id"`
	UserId       string `json:"user_id"`
	ProfileId    string
	OrgNamespace string
}

type SessionOptions struct {
	ApiVersion    string
	Alias         string
	RefreshMethod RefreshMethod
}

type OAuthError struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type ForceSession struct {
	AccessToken    string `json:"access_token"`
	InstanceUrl    string `json:"instance_url"`
	IssuedAt       string `json:"issued_at"`
	Scope          string `json:"scope"`
	ClientId       string
	RefreshToken   string
	ForceEndpoint  ForceEndpoint
	EndpointUrl    string `json:"endpoint_url"`
	UserInfo       *UserInfo
	SessionOptions *SessionOptions
}

type LoginFault struct {
	ExceptionCode    string `xml:"exceptionCode" json:"exceptionCode"`
	ExceptionMessage string `xml:"exceptionMessage" json:"exceptionCode"`
}

type SoapFault struct {
	FaultCode   string     `xml:"Body>Fault>faultcode"`
	FaultString string     `xml:"Body>Fault>faultstring"`
	Detail      LoginFault `xml:"Body>Fault>detail>LoginFault"`
}

type GenericForceError struct {
	Error_Description string
	Error             string
}

type ForceError struct {
	Message   string
	ErrorCode string
}

type FieldName struct {
	FieldName string
	IsObject  bool
}

type SelectStruct struct {
	ObjectName string
	FieldNames []FieldName
}

type ForceEndpoint int

type ForceSobject map[string]interface{}

type ForceCreateRecordResult struct {
	Errors  []string
	Id      string
	Success bool
}

type ForceLimits map[string]ForceLimit

type ForceLimit struct {
	Name      string
	Remaining int64
	Max       int64
}

type ForcePasswordStatusResult struct {
	IsExpired bool
}

type ForcePasswordResetResult struct {
	NewPassword string
}

type ForceRecord map[string]interface{}

type ForceQueryResult struct {
	Done           bool
	Records        []ForceRecord
	TotalSize      int
	NextRecordsUrl string
}

type ForceSobjectsResult struct {
	Encoding     string
	MaxBatchSize int
	Sobjects     []ForceSobject
}

type Result struct {
	Id      string
	Success bool
	Created bool
	Message string
}

type QueryOptions struct {
	IsTooling bool
	QueryAll  bool
}

type AuraDefinitionBundleResult struct {
	Done           bool
	Records        []ForceRecord
	TotalSize      int
	QueryLocator   string
	Size           int
	EntityTypeName string
	NextRecordsUrl string
}

type AuraDefinitionBundle struct {
	Id               string
	IsDeleted        bool
	DeveloperName    string
	Language         string
	MasterLabel      string
	NamespacePrefix  string
	CreatedDate      string
	CreatedById      string
	LastModifiedDate string
	LastModifiedById string
	SystemModstamp   string
	ApiVersion       int
	Description      string
}

type AuraDefinition struct {
	Id                     string
	IsDeleted              bool
	CreatedDate            string
	CreatedById            string
	LastModifiedDate       string
	LastModifiedById       string
	SystemModstamp         string
	AuraDefinitionBundleId string
	DefType                string
	Format                 string
	Source                 string
}

type ComponentFile struct {
	FileName    string
	ComponentId string
}

type BundleManifest struct {
	Name  string
	Id    string
	Files []ComponentFile
}

func NewForce(creds *ForceSession) (force *Force) {
	force = new(Force)
	force.Credentials = creds
	force.Metadata = NewForceMetadata(force)
	force.Partner = NewForcePartner(force)
	return
}

func endpointUrl(endpoint ForceEndpoint) string {
	switch endpoint {
	case EndpointProduction:
		return "https://login.salesforce.com"
	case EndpointTest:
		return "https://test.salesforce.com"
	case EndpointPrerelease:
		return "https://prerelna1.pre.salesforce.com"
	case EndpointMobile1:
		return "https://mobile1.t.pre.salesforce.com"
	case EndpointCustom:
		fallthrough
	default:
		Log.Info("Deprecated use of CustomEndpoint")
		return CustomEndpoint
	}
}

func ForceSoapLogin(endpoint ForceEndpoint, username string, password string) (creds ForceSession, err error) {
	Log.Info("Deprecated call to ForceSoapLogin.  Use ForceSoapLoginAtEndpoint.")
	url := endpointUrl(endpoint)
	return ForceSoapLoginAtEndpoint(url, username, password)
}

func ForceSoapLoginAtEndpoint(endpoint string, username string, password string) (creds ForceSession, err error) {
	var surl string
	version := strings.Split(apiVersion, "v")[1]
	if endpoint == "" {
		ErrorAndExit("Unable to login with SOAP. Unknown endpoint type")
	}
	surl = fmt.Sprintf("%s/services/Soap/u/%s", endpoint, version)

	soap := NewSoap(surl, "", "")
	response, err := soap.ExecuteLogin(username, password)
	if err != nil {
		ErrorAndExit(err.Error())
	}
	var result struct {
		SessionId    string `xml:"Body>loginResponse>result>sessionId"`
		Id           string `xml:"Body>loginResponse>result>userId"`
		Instance_url string `xml:"Body>loginResponse>result>serverUrl"`
	}
	var fault SoapFault
	if err = xml.Unmarshal(response, &fault); fault.Detail.ExceptionMessage != "" {
		ErrorAndExit(fault.Detail.ExceptionCode + ": " + fault.Detail.ExceptionMessage)
	}
	if err = xml.Unmarshal(response, &result); err != nil {
		return
	}
	orgid := strings.Split(result.SessionId, "!")[0]
	u, err := url.Parse(result.Instance_url)
	if err != nil {
		return
	}
	instanceUrl := u.Scheme + "://" + u.Host
	creds = ForceSession{
		AccessToken: result.SessionId,
		InstanceUrl: instanceUrl,
		EndpointUrl: endpoint,
		UserInfo: &UserInfo{
			OrgId:  orgid,
			UserId: result.Id,
		},
		SessionOptions: &SessionOptions{
			ApiVersion: apiVersionNumber,
		},
	}
	return
}

func tokenURL(endpoint string) (tokenURL string) {
	return fmt.Sprintf("%s/services/oauth2/token", endpoint)
}

func (f *Force) refreshTokenURL() string {
	return tokenURL(f.Credentials.EndpointUrl)
}

func ForceLogin(endpoint ForceEndpoint) (creds ForceSession, err error) {
	Log.Info("Deprecated call to ForceLogin.  Use ForceLogin.")
	url := endpointUrl(endpoint)
	return ForceLoginAtEndpoint(url)
}

func ForceLoginAtEndpoint(endpoint string) (creds ForceSession, err error) {
	ch := make(chan ForceSession)
	port, err := startLocalHttpServer(ch)
	var url string

	Redir := RedirectUri

	url = fmt.Sprintf("%s/services/oauth2/authorize?response_type=token&client_id=%s&redirect_uri=%s&state=%d&prompt=login", endpoint, ClientId, Redir, port)

	err = desktop.Open(url)
	creds = <-ch
	if creds.RefreshToken != "" {
		creds.SessionOptions = &SessionOptions{
			RefreshMethod: RefreshOauth,
		}
	}
	creds.EndpointUrl = endpoint
	creds.ClientId = ClientId
	return
}

func (f *Force) GetCodeCoverage(classId string, className string) (err error) {
	url := fmt.Sprintf("%s/services/data/%s/query/?q=Select+Id+From+ApexClass+Where+Name+=+'%s'", f.Credentials.InstanceUrl, apiVersion, className)

	body, err := f.httpGet(url)
	if err != nil {
		return
	}
	var result ForceQueryResult
	json.Unmarshal(body, &result)

	if len(result.Records) == 0 {
		return ClassNotFoundError
	}

	classId = result.Records[0]["Id"].(string)
	url = fmt.Sprintf("%s/services/data/%s/tooling/query/?q=Select+Coverage,+NumLinesCovered,+NumLinesUncovered,+ApexTestClassId,+ApexClassorTriggerId+From+ApexCodeCoverage+Where+ApexClassorTriggerId='%s'", f.Credentials.InstanceUrl, apiVersion, classId)

	body, err = f.httpGet(url)
	if err != nil {
		return
	}

	//var result ForceSobjectsResult
	json.Unmarshal(body, &result)

	if len(result.Records) == 0 {
		return MetricsNotFoundError
	}

	Log.Info(fmt.Sprintf("\n%d lines covered\n%d lines not covered\n", int(result.Records[0]["NumLinesCovered"].(float64)), int(result.Records[0]["NumLinesUncovered"].(float64))))
	return
}

func (f *Force) DeleteDataPipeline(id string) (err error) {
	url := fmt.Sprintf("%s/services/data/%s/tooling/sobjects/DataPipeline/%s", f.Credentials.InstanceUrl, apiVersion, id)
	_, err = f.httpDelete(url)
	return
}

func (f *Force) UpdateDataPipeline(id string, masterLabel string, scriptContent string) (err error) {
	url := fmt.Sprintf("%s/services/data/%s/tooling/sobjects/DataPipeline/%s", f.Credentials.InstanceUrl, apiVersion, id)
	attrs := make(map[string]string)
	attrs["MasterLabel"] = masterLabel
	attrs["ScriptContent"] = scriptContent

	_, err = f.httpPatch(url, attrs)
	return
}

func (f *Force) CreateDataPipeline(name string, masterLabel string, apiVersionNumber string, scriptContent string, scriptType string) (result ForceCreateRecordResult, err error, emessages []ForceError) {
	aurl := fmt.Sprintf("%s/services/data/%s/tooling/sobjects/DataPipeline", f.Credentials.InstanceUrl, apiVersion)

	attrs := make(map[string]string)
	attrs["DeveloperName"] = name
	attrs["ScriptType"] = scriptType
	attrs["MasterLabel"] = masterLabel
	attrs["ApiVersion"] = apiVersionNumber
	attrs["ScriptContent"] = scriptContent

	body, err, emessages := f.httpPost(aurl, attrs)
	if err != nil {
		return
	}
	json.Unmarshal(body, &result)

	return

}

func (f *Force) CreateDataPipelineJob(id string) (result ForceCreateRecordResult, err error, emessages []ForceError) {
	aurl := fmt.Sprintf("%s/services/data/%s/tooling/sobjects/DataPipelineJob", f.Credentials.InstanceUrl, apiVersion)

	attrs := make(map[string]string)
	attrs["DataPipelineId"] = id

	body, err, emessages := f.httpPost(aurl, attrs)
	if err != nil {
		return
	}
	json.Unmarshal(body, &result)

	return

}

func (f *Force) GetDataPipeline(name string) (results ForceQueryResult, err error) {

	query := fmt.Sprintf("SELECT Id, MasterLabel, DeveloperName, ScriptContent, ScriptType FROM DataPipeline Where DeveloperName = '%s'", name)
	results, err = f.QueryDataPipeline(query)

	return

}

func (f *Force) QueryDataPipeline(soql string) (results ForceQueryResult, err error) {
	aurl := fmt.Sprintf("%s/services/data/%s/tooling/query?q=%s", f.Credentials.InstanceUrl, apiVersion,
		url.QueryEscape(soql))

	body, err := f.httpGet(aurl)
	if err != nil {
		return
	}
	json.Unmarshal(body, &results)

	return

}

func (f *Force) QueryDataPipelineJob(soql string) (results ForceQueryResult, err error) {
	aurl := fmt.Sprintf("%s/services/data/%s/tooling/query?q=%s", f.Credentials.InstanceUrl, apiVersion,
		url.QueryEscape(soql))

	body, err := f.httpGet(aurl)
	if err != nil {
		return
	}
	json.Unmarshal(body, &results)

	return

}

func (f *Force) GetAuraBundles() (bundles AuraDefinitionBundleResult, definitions AuraDefinitionBundleResult, err error) {
	bundles, err = f.GetAuraBundlesList()
	definitions, err = f.GetAuraBundleDefinitions()
	return
}

func (f *Force) GetAuraBundleDefinitions() (definitions AuraDefinitionBundleResult, err error) {
	aurl := fmt.Sprintf("%s/services/data/%s/tooling/query?q=%s", f.Credentials.InstanceUrl, apiVersion,
		url.QueryEscape("SELECT Id, Source, AuraDefinitionBundleId, DefType, Format FROM AuraDefinition"))

	body, err := f.httpGet(aurl)
	if err != nil {
		return
	}
	json.Unmarshal(body, &definitions)

	if !definitions.Done {
		f.GetMoreAuraBundleDefinitions(&definitions)
	}

	return
}

func (f *Force) GetMoreAuraBundleDefinitions(definitions *AuraDefinitionBundleResult) (err error) {

	isDone := definitions.Done
	nextRecordsUrl := definitions.NextRecordsUrl

	for !isDone {

		moreDefs := new(AuraDefinitionBundleResult)
		aurl := fmt.Sprintf("%s%s", f.Credentials.InstanceUrl, nextRecordsUrl)

		body, err := f.httpGet(aurl)
		if err != nil {
			return err
		}
		json.Unmarshal(body, &moreDefs)

		definitions.Done = moreDefs.Done
		definitions.Records = append(definitions.Records, moreDefs.Records...)

		isDone = moreDefs.Done

		if !isDone {
			nextRecordsUrl = moreDefs.NextRecordsUrl
		}
	}

	return
}

func (f *Force) GetAuraBundlesList() (bundles AuraDefinitionBundleResult, err error) {
	aurl := fmt.Sprintf("%s/services/data/%s/tooling/query?q=%s", f.Credentials.InstanceUrl, apiVersion,
		url.QueryEscape("SELECT Id, DeveloperName, NamespacePrefix, ApiVersion, Description FROM AuraDefinitionBundle"))
	body, err := f.httpGet(aurl)
	if err != nil {
		return
	}
	json.Unmarshal(body, &bundles)

	return
}

func (f *Force) GetAuraBundle(bundleName string) (bundles AuraDefinitionBundleResult, definitions AuraDefinitionBundleResult, err error) {
	bundles, err = f.GetAuraBundleByName(bundleName)
	if len(bundles.Records) == 0 {
		ErrorAndExit(fmt.Sprintf("There is no Aura bundle named %q", bundleName))
	}
	bundle := bundles.Records[0]
	definitions, err = f.GetAuraBundleDefinition(bundle["Id"].(string))
	return
}

func (f *Force) GetAuraBundleByName(bundleName string) (bundles AuraDefinitionBundleResult, err error) {
	criteria := fmt.Sprintf(" Where DeveloperName = '%s'", bundleName)

	aurl := fmt.Sprintf("%s/services/data/%s/tooling/query?q=%s", f.Credentials.InstanceUrl, apiVersion,
		url.QueryEscape(fmt.Sprintf("SELECT Id, DeveloperName, NamespacePrefix, ApiVersion, Description FROM AuraDefinitionBundle%s", criteria)))

	body, err := f.httpGet(aurl)
	if err != nil {
		return
	}
	json.Unmarshal(body, &bundles)

	return
}

func (f *Force) GetAuraBundleDefinition(id string) (definitions AuraDefinitionBundleResult, err error) {
	aurl := fmt.Sprintf("%s/services/data/%s/tooling/query?q=%s", f.Credentials.InstanceUrl, apiVersion,
		url.QueryEscape(fmt.Sprintf("SELECT Id, Source, AuraDefinitionBundleId, DefType, Format FROM AuraDefinition WHERE AuraDefinitionBundleId = '%s'", id)))

	body, err := f.httpGet(aurl)
	if err != nil {
		return
	}
	json.Unmarshal(body, &definitions)

	return
}

func (f *Force) CreateAuraBundle(bundleName string) (result ForceCreateRecordResult, err error, emessages []ForceError) {
	aurl := fmt.Sprintf("%s/services/data/%s/tooling/sobjects/AuraDefinitionBundle", f.Credentials.InstanceUrl, apiVersion)
	attrs := make(map[string]string)
	attrs["DeveloperName"] = bundleName
	attrs["Description"] = "An Aura Bundle"
	attrs["MasterLabel"] = bundleName
	attrs["ApiVersion"] = apiVersionNumber
	body, err, emessages := f.httpPost(aurl, attrs)
	if err != nil {
		return
	}
	json.Unmarshal(body, &result)

	return
}

func (f *Force) CreateAuraComponent(attrs map[string]string) (result ForceCreateRecordResult, err error, emessages []ForceError) {
	aurl := fmt.Sprintf("%s/services/data/%s/tooling/sobjects/AuraDefinition", f.Credentials.InstanceUrl, apiVersion)
	body, err, emessages := f.httpPost(aurl, attrs)
	if err != nil {
		return
	}
	json.Unmarshal(body, &result)

	return
}

func (f *Force) ListSobjects() (sobjects []ForceSobject, err error) {
	url := fmt.Sprintf("%s/services/data/%s/sobjects", f.Credentials.InstanceUrl, apiVersion)
	body, err := f.httpGet(url)
	if err != nil {
		return
	}
	var result ForceSobjectsResult
	json.Unmarshal(body, &result)
	sobjects = result.Sobjects
	return
}

func (f *Force) GetSobject(name string) (sobject ForceSobject, err error) {
	url := fmt.Sprintf("%s/services/data/%s/sobjects/%s/describe", f.Credentials.InstanceUrl, apiVersion, name)
	body, err := f.httpGet(url)
	if err != nil {
		return
	}
	json.Unmarshal(body, &sobject)
	return
}

func (f *Force) QueryAndSend(qs string, processor chan<- ForceRecord, options ...func(*QueryOptions)) error {
	defer func() {
		close(processor)
	}()

	qopts := f.legacyQueryOptions(qs, options...)
	err := query.Query(func(records []query.Record) bool {
		for _, row := range records {
			processor <- row.Raw
		}
		return true
	}, qopts...)
	return err
}

func (f *Force) Query(qs string, options ...func(*QueryOptions)) (ForceQueryResult, error) {
	qopts := f.legacyQueryOptions(qs, options...)
	result := ForceQueryResult{}
	records, err := query.Eager(qopts...)
	if err != nil {
		return result, err
	}
	result.Done = true
	result.TotalSize = len(records)
	result.Records = make([]ForceRecord, len(records))
	for i, r := range records {
		// NOTE: This will keep intact the subquery locator bug
		result.Records[i] = r.Raw
	}
	return result, err
}

func (f *Force) QueryOptions() []query.Option {
	return []query.Option{
		query.HttpGet(f.httpGet),
		query.InstanceUrl(f.Credentials.InstanceUrl),
		query.ApiVersion(apiVersion),
	}
}

func (f *Force) legacyQueryOptions(qs string, options ...func(*QueryOptions)) []query.Option {
	qopts := f.QueryOptions()
	qopts = append(qopts, query.QS(qs))

	queryOptions := QueryOptions{}
	for _, option := range options {
		option(&queryOptions)
	}
	if queryOptions.QueryAll {
		qopts = append(qopts, query.All)
	}
	if queryOptions.IsTooling {
		qopts = append(qopts, query.Tooling)
	}
	return qopts
}

func (f *Force) Get(url string) (object ForceRecord, err error) {
	body, err := f.httpGet(url)
	if err != nil {
		return
	}
	err = json.Unmarshal(body, &object)
	return
}

func (f *Force) GetLimits() (result map[string]ForceLimit, err error) {

	url := fmt.Sprintf("%s/services/data/%s/limits", f.Credentials.InstanceUrl, apiVersion)
	body, err := f.httpGet(url)
	if err != nil {
		return
	}
	err = json.Unmarshal([]byte(body), &result)
	return

}

func (f *Force) GetPasswordStatus(id string) (result ForcePasswordStatusResult, err error) {
	url := fmt.Sprintf("%s/services/data/%s/sobjects/User/%s/password", f.Credentials.InstanceUrl, apiVersion, id)
	body, err := f.httpGet(url)
	if err != nil {
		return
	}
	err = json.Unmarshal(body, &result)
	return
}

func (f *Force) ResetPassword(id string) (result ForcePasswordResetResult, err error) {
	url := fmt.Sprintf("%s/services/data/%s/sobjects/User/%s/password", f.Credentials.InstanceUrl, apiVersion, id)
	body, err := f.httpDelete(url)
	if err != nil {
		return
	}
	err = json.Unmarshal(body, &result)
	return
}

func (f *Force) ChangePassword(id string, attrs map[string]string) (result string, err error, emessages []ForceError) {
	url := fmt.Sprintf("%s/services/data/%s/sobjects/User/%s/password", f.Credentials.InstanceUrl, apiVersion, id)
	_, err, emessages = f.httpPost(url, attrs)
	return
}

func (f *Force) GetRecord(sobject, id string) (object ForceRecord, err error) {
	fields := strings.Split(id, ":")
	var url string
	if len(fields) == 1 {
		url = fmt.Sprintf("%s/services/data/%s/sobjects/%s/%s", f.Credentials.InstanceUrl, apiVersion, sobject, id)
	} else {
		url = fmt.Sprintf("%s/services/data/%s/sobjects/%s/%s/%s", f.Credentials.InstanceUrl, apiVersion, sobject, fields[0], fields[1])
	}

	body, err := f.httpGet(url)
	if err != nil {
		return
	}
	err = json.Unmarshal(body, &object)
	return
}

func (f *Force) CreateRecord(sobject string, attrs map[string]string) (id string, err error, emessages []ForceError) {
	url := fmt.Sprintf("%s/services/data/%s/sobjects/%s", f.Credentials.InstanceUrl, apiVersion, sobject)
	body, err, emessages := f.httpPost(url, attrs)
	var result ForceCreateRecordResult
	json.Unmarshal(body, &result)
	id = result.Id
	return
}

func (f *Force) SetFLS(profileId string, objectName string, fieldName string) {
	// First, write out a file to a temporary location with a package.xml

	f.Metadata.UpdateFLSOnProfile(objectName, fieldName)
}

func (f *Force) QueryProfile(fields ...string) (results ForceQueryResult, err error) {

	url := fmt.Sprintf("%s/services/data/%s/tooling/query?q=Select+%s+From+Profile+Where+Id='%s'",
		f.Credentials.InstanceUrl,
		apiVersion,
		strings.Join(fields, ","),
		f.Credentials.UserInfo.ProfileId)

	body, err := f.httpGet(url)
	if err != nil {
		return
	}
	json.Unmarshal(body, &results)
	return
}

func (f *Force) QueryTraceFlags() (results ForceQueryResult, err error) {
	url := fmt.Sprintf("%s/services/data/%s/tooling/query/?q=Select+Id,+DebugLevel.DeveloperName,++ApexCode,+ApexProfiling,+Callout,+CreatedDate,+Database,+ExpirationDate,+System,+TracedEntity.Name,+Validation,+Visualforce,+Workflow+From+TraceFlag+Order+By+ExpirationDate,TracedEntity.Name", f.Credentials.InstanceUrl, apiVersion)
	body, err := f.httpGet(url)
	if err != nil {
		return
	}
	json.Unmarshal(body, &results)
	return
}

func (f *Force) QueryDefaultDebugLevel() (id string, err error) {
	url := fmt.Sprintf("%s/services/data/%s/tooling/query/?q=Select+Id+From+DebugLevel+Where+DeveloperName+=+'Force_CLI'", f.Credentials.InstanceUrl, apiVersion)
	body, err := f.httpGet(url)
	if err != nil {
		return
	}
	var results ForceQueryResult
	json.Unmarshal(body, &results)
	if len(results.Records) == 1 {
		id = results.Records[0]["Id"].(string)
	}
	return
}

func (f *Force) DefaultDebugLevel() (id string, err error, emessages []ForceError) {
	id, err = f.QueryDefaultDebugLevel()
	if err != nil || id != "" {
		return
	}
	url := fmt.Sprintf("%s/services/data/%s/tooling/sobjects/DebugLevel", f.Credentials.InstanceUrl, apiVersion)

	// The log levels are currently hard-coded to a useful level of logging
	// without hitting the maximum log size of 2MB in most cases, hopefully.
	attrs := make(map[string]string)
	attrs["ApexCode"] = "Fine"
	attrs["ApexProfiling"] = "Error"
	attrs["Callout"] = "Info"
	attrs["Database"] = "Info"
	attrs["System"] = "Info"
	attrs["Validation"] = "Warn"
	attrs["Visualforce"] = "Info"
	attrs["Workflow"] = "Info"
	attrs["DeveloperName"] = "Force_CLI"
	attrs["MasterLabel"] = "Force_CLI"

	body, err, emessages := f.httpPost(url, attrs)
	if err != nil {
		return
	}
	var result ForceCreateRecordResult
	json.Unmarshal(body, &result)
	if result.Success {
		id = result.Id
	}

	return
}

func (f *Force) StartTrace(userId ...string) (result ForceCreateRecordResult, err error, emessages []ForceError) {
	debugLevel, err, emessages := f.DefaultDebugLevel()
	if err != nil {
		return
	}
	url := fmt.Sprintf("%s/services/data/%s/tooling/sobjects/TraceFlag", f.Credentials.InstanceUrl, apiVersion)

	attrs := make(map[string]string)
	attrs["DebugLevelId"] = debugLevel
	if len(userId) == 1 {
		attrs["TracedEntityId"] = userId[0]
		attrs["LogType"] = "USER_DEBUG"
	} else {
		attrs["TracedEntityId"] = f.Credentials.UserInfo.UserId
		attrs["LogType"] = "DEVELOPER_LOG"
	}

	body, err, emessages := f.httpPost(url, attrs)
	if err != nil {
		return
	}
	json.Unmarshal(body, &result)

	return
}

func (f *Force) RetrieveLog(logId string) (result string, err error) {
	url := fmt.Sprintf("%s/services/data/%s/tooling/sobjects/ApexLog/%s/Body", f.Credentials.InstanceUrl, apiVersion, logId)
	body, err := f.httpGet(url)
	result = string(body)
	return
}

func (f *Force) QueryLogs() (results ForceQueryResult, err error) {
	url := fmt.Sprintf("%s/services/data/%s/tooling/query/?q=Select+Id,+Application,+DurationMilliseconds,+Location,+LogLength,+LogUser.Name,+Operation,+Request,StartTime,+Status+From+ApexLog+Order+By+StartTime", f.Credentials.InstanceUrl, apiVersion)
	body, err := f.httpGet(url)
	if err != nil {
		return
	}
	json.Unmarshal(body, &results)
	return
}

func (f *Force) RetrieveEventLogFile(elfId string) (result string, err error) {
	url := fmt.Sprintf("%s/services/data/%s/sobjects/EventLogFile/%s/LogFile", f.Credentials.InstanceUrl, apiVersion, elfId)
	body, err := f.httpGet(url)
	if err != nil {
		return
	}
	result = string(body)
	return
}

func (force *Force) useHourlyLogs() bool {
	const EventLogFile string = "EventLogFile"
	const HourlyEnabledField string = "Sequence"
	sobject, err := force.GetSobject(EventLogFile)
	if err != nil {
		ErrorAndExit(err.Error())
	}
	fields := ForceSobjectFields(sobject["fields"].([]interface{}))
	for _, f := range fields {
		field := f.(map[string]interface{})
		if field["name"] == HourlyEnabledField {
			return true
		}
	}
	return false
}

func (f *Force) QueryEventLogFiles() (results ForceQueryResult, err error) {
	url := ""
	currApi, e := strconv.ParseFloat(f.Credentials.SessionOptions.ApiVersion, 64)
	if e != nil {
		ErrorAndExit(e.Error())
	}
	if f.useHourlyLogs() && currApi >= 37.0 {
		url = fmt.Sprintf("%s/services/data/%s/query/?q=Select+Id,+LogDate,+EventType,+LogFileLength,+Sequence,+Interval+FROM+EventLogFile+ORDER+BY+LogDate+DESC,+EventType,+Sequence,+Interval", f.Credentials.InstanceUrl, apiVersion)
	} else {
		url = fmt.Sprintf("%s/services/data/%s/query/?q=Select+Id,+LogDate,+EventType,+LogFileLength+FROM+EventLogFile+ORDER+BY+LogDate+DESC,+EventType", f.Credentials.InstanceUrl, apiVersion)
	}
	body, err := f.httpGet(url)
	if err != nil {
		return
	}
	json.Unmarshal(body, &results)
	return
}

func (f *Force) UpdateAuraComponent(source map[string]string, id string) (err error) {
	url := fmt.Sprintf("%s/services/data/%s/tooling/sobjects/AuraDefinition/%s", f.Credentials.InstanceUrl, apiVersion, id)
	_, err = f.httpPatch(url, source)
	return
}

func (f *Force) DeleteToolingRecord(objecttype string, id string) (err error) {
	url := fmt.Sprintf("%s/services/data/%s/tooling/sobjects/%s/%s", f.Credentials.InstanceUrl, apiVersion, objecttype, id)
	_, err = f.httpDelete(url)
	return
}

func (f *Force) CreateToolingRecord(objecttype string, attrs map[string]string) (result ForceCreateRecordResult, err error) {
	aurl := fmt.Sprintf("%s/services/data/%s/tooling/sobjects/%s", f.Credentials.InstanceUrl, apiVersion, objecttype)
	body, err, _ := f.httpPost(aurl, attrs)

	if err != nil {
		return
	}
	json.Unmarshal(body, &result)

	return
}

func (f *Force) DescribeSObject(objecttype string) (result string, err error) {
	url := fmt.Sprintf("%s/services/data/%s/sobjects/%s/describe", f.Credentials.InstanceUrl, apiVersion, objecttype)
	body, err := f.httpGet(url)
	if err != nil {
		return
	}
	result = string(body)
	return
}

func (f *Force) UpdateRecord(sobject string, id string, attrs map[string]string) (err error) {
	fields := strings.Split(id, ":")
	var url string
	if len(fields) == 1 {
		url = fmt.Sprintf("%s/services/data/%s/sobjects/%s/%s", f.Credentials.InstanceUrl, apiVersion, sobject, id)
	} else {
		url = fmt.Sprintf("%s/services/data/%s/sobjects/%s/%s/%s", f.Credentials.InstanceUrl, apiVersion, sobject, fields[0], fields[1])
	}
	_, err = f.httpPatch(url, attrs)
	return
}

func (f *Force) DeleteRecord(sobject string, id string) (err error) {
	url := fmt.Sprintf("%s/services/data/%s/sobjects/%s/%s", f.Credentials.InstanceUrl, apiVersion, sobject, id)
	_, err = f.httpDelete(url)
	return
}

func (f *Force) Whoami() (me ForceRecord, err error) {
	me, err = f.GetRecord("User", f.Credentials.UserInfo.UserId)
	return
}

// Prepend /services/data/vXX.0 to URL
func (f *Force) fullRestUrl(url string) string {
	return fmt.Sprintf("/services/data/%s/%s", apiVersion, strings.TrimLeft(url, "/"))
}

// Prepend https schema and instance to URL
func (f *Force) qualifyUrl(url string) string {
	return fmt.Sprintf("%s/%s", f.Credentials.InstanceUrl, strings.TrimLeft(url, "/"))
}

func (f *Force) GetAbsoluteBytes(url string) (result []byte, err error) {
	qualifiedUrl := f.qualifyUrl(url)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", f.Credentials.AccessToken),
	}
	body, _, err := f.httpGetRequest(qualifiedUrl, headers)
	if err == SessionExpiredError {
		err = f.RefreshSession()
		if err != nil {
			return
		}
		return f.GetAbsoluteBytes(url)
	}
	return body, err
}

func (f *Force) GetAbsolute(url string) (string, error) {
	data, err := f.GetAbsoluteBytes(url)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (f *Force) GetREST(url string) (result string, err error) {
	fullUrl := f.fullRestUrl(url)
	return f.GetAbsolute(fullUrl)
}

func (f *Force) PostPatchAbsolute(url string, content string, method string) (result string, err error) {
	if method == "POST" {
		return f.PostAbsolute(url, content)
	} else {
		return f.PatchAbsolute(url, content)
	}
}

func (f *Force) PostPatchREST(url string, content string, method string) (result string, err error) {
	if method == "POST" {
		return f.PostREST(url, content)
	} else {
		return f.PatchREST(url, content)
	}
}

func (f *Force) PostAbsolute(url string, content string) (result string, err error) {
	qualifiedUrl := f.qualifyUrl(url)
	body, err := f.httpPostJSON(qualifiedUrl, content)
	if err == SessionExpiredError {
		err = f.RefreshSession()
		if err != nil {
			return
		}
		return f.PostREST(url, content)
	}
	result = string(body)
	return
}

func (f *Force) PostREST(url string, content string) (result string, err error) {
	fullUrl := f.fullRestUrl(url)
	return f.PostAbsolute(fullUrl, content)
}

func (f *Force) PatchAbsolute(url string, content string) (result string, err error) {
	qualifiedUrl := f.qualifyUrl(url)
	body, err := f.httpPatchJSON(qualifiedUrl, content)
	if err == SessionExpiredError {
		err = f.RefreshSession()
		if err != nil {
			return
		}
		return f.PatchREST(url, content)
	}
	result = string(body)
	return
}

func (f *Force) PatchREST(url string, content string) (result string, err error) {
	fullUrl := f.fullRestUrl(url)
	return f.PatchAbsolute(fullUrl, content)
}

func (f *Force) getForceResult(url string) (results ForceQueryResult, err error) {
	body, err := f.httpGet(fmt.Sprintf("%s%s", f.Credentials.InstanceUrl, url))
	if err != nil {
		return
	}
	json.Unmarshal(body, &results)
	return
}

func (f *Force) httpGet(url string) (body []byte, err error) {
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", f.Credentials.AccessToken),
	}
	body, _, err = f.httpGetRequest(url, headers)
	if err == SessionExpiredError {
		err = f.RefreshSession()
		if err != nil {
			return
		}
		return f.httpGet(url)
	}
	return
}

func (f *Force) httpGetBulk(url string) (body []byte, contentType string, err error) {
	headers := map[string]string{
		"X-SFDC-Session": fmt.Sprintf("Bearer %s", f.Credentials.AccessToken),
		"Content-Type":   "application/xml",
	}
	body, contentType, err = f.httpGetRequest(url, headers)
	if err == SessionExpiredError {
		err = f.RefreshSession()
		if err != nil {
			return
		}
		return f.httpGetBulk(url)
	}
	return
}

func (f *Force) httpGetBulkAndSend(url string, results chan<- BatchResultChunk) (err error) {
	headers := map[string]string{
		"X-SFDC-Session": fmt.Sprintf("Bearer %s", f.Credentials.AccessToken),
		"Content-Type":   "application/xml",
	}
	err = f.httpGetRequestAndSend(url, headers, results)
	if err == SessionExpiredError {
		err = f.RefreshSession()
		if err != nil {
			return
		}
		return f.httpGetBulkAndSend(url, results)
	}
	return
}

func (f *Force) httpGetBulkJSON(url string) (body []byte, err error) {
	headers := map[string]string{
		"X-SFDC-Session": fmt.Sprintf("Bearer %s", f.Credentials.AccessToken),
		"Content-Type":   "application/json",
	}
	body, _, err = f.httpGetRequest(url, headers)
	if err == SessionExpiredError {
		err = f.RefreshSession()
		if err != nil {
			return
		}
		return f.httpGetBulkJSON(url)
	}
	return
}

func (f *Force) httpGetBulkJSONAndSend(url string, results chan<- BatchResultChunk) (err error) {
	headers := map[string]string{
		"X-SFDC-Session": fmt.Sprintf("Bearer %s", f.Credentials.AccessToken),
		"Content-Type":   "application/json",
	}
	err = f.httpGetRequestAndSend(url, headers, results)
	if err == SessionExpiredError {
		err = f.RefreshSession()
		if err != nil {
			return
		}
		return f.httpGetBulkJSONAndSend(url, results)
	}
	return
}

func (f *Force) httpGetRequest(url string, headers map[string]string) (body []byte, contentType string, err error) {
	req, err := httpRequest("GET", url, nil)
	if err != nil {
		return
	}
	for headerName, headerValue := range headers {
		req.Header.Add(headerName, headerValue)
	}
	res, err := doRequest(req)
	if err != nil {
		return
	}
	defer res.Body.Close()

	body, err = ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}

	contentType = res.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/xml") {
		contentType = "XML"
		if res.StatusCode/100 != 2 {
			var fault LoginFault
			xml.Unmarshal(body, &fault)
			if fault.ExceptionCode == "InvalidSessionId" {
				err = SessionExpiredError
			}
		}
	} else {
		contentType = "JSON"
		if res.StatusCode/100 != 2 {
			var messages []ForceError
			json.Unmarshal(body, &messages)
			switch {
			case len(messages) == 0:
				err = errors.New(string(body))
			case messages[0].ErrorCode == "REQUEST_LIMIT_EXCEEDED":
				err = APILimitExceededError
			case messages[0].ErrorCode == "INVALID_OPERATION":
				// e.g. "The requested operation is not yet supported by this
				// SObject storage type, contact salesforce.com support for more
				// information."
				err = &APIError{"Invalid Operation: " + messages[0].Message}
			default:
				err = errors.New(messages[0].Message)
			}
		}
	}

	_, isAPIError := err.(*APIError)
	if res.StatusCode == 401 || (res.StatusCode == 403 && !isAPIError) {
		err = SessionExpiredError
	}

	return
}

func (f *Force) httpGetRequestAndSend(url string, headers map[string]string, results chan<- BatchResultChunk) (err error) {
	req, err := httpRequest("GET", url, nil)
	if err != nil {
		return err
	}
	for headerName, headerValue := range headers {
		req.Header.Add(headerName, headerValue)
	}
	res, err := doRequest(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == 401 || res.StatusCode == 403 {
		return SessionExpiredError
	}
	contentType := res.Header.Get("Content-Type")
	if res.StatusCode/100 != 2 {
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}
		if strings.HasPrefix(contentType, "application/xml") {
			var fault LoginFault
			xml.Unmarshal(body, &fault)
			if fault.ExceptionCode == "InvalidSessionId" {
				err = SessionExpiredError
			}
		} else {
			var messages []ForceError
			json.Unmarshal(body, &messages)
			if len(messages) > 0 {
				err = errors.New(messages[0].Message)
			} else {
				err = errors.New(string(body))
			}
		}
		return err
	}

	buf := make([]byte, 50*1024*1024)
	firstChunk := true
	isCSV := strings.Contains(contentType, "text/csv")
	for {
		n, err := io.ReadFull(res.Body, buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			results <- BatchResultChunk{
				HasCSVHeader: firstChunk && isCSV,
				Data:         data,
			}
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		} else if err != nil {
			return err
		}
		firstChunk = false
	}

	return nil
}

func (f *Force) httpPostCSV(url string, data string, requestOptions ...func(*http.Request)) (body []byte, err error) {
	body, err = f.httpPostWithContentType(url, data, "text/csv", requestOptions...)
	if err == SessionExpiredError {
		err = f.RefreshSession()
		if err != nil {
			return
		}
		return f.httpPostCSV(url, data, requestOptions...)
	}
	return
}

func (f *Force) httpPostXML(url string, data string, requestOptions ...func(*http.Request)) (body []byte, err error) {
	body, err = f.httpPostWithContentType(url, data, "application/xml", requestOptions...)
	if err == SessionExpiredError {
		err = f.RefreshSession()
		if err != nil {
			return
		}
		return f.httpPostXML(url, data, requestOptions...)
	}
	return
}

func (f *Force) httpPostJSON(url string, data string, requestOptions ...func(*http.Request)) (body []byte, err error) {
	body, err = f.httpPostWithContentType(url, data, "application/json", requestOptions...)
	if err == SessionExpiredError {
		err = f.RefreshSession()
		if err != nil {
			return
		}
		return f.httpPostJSON(url, data, requestOptions...)
	}
	return
}

func (f *Force) httpPatchJSON(url string, data string) (body []byte, err error) {
	body, err = f.httpPatchWithContentType(url, data, "application/json")
	if err == SessionExpiredError {
		err = f.RefreshSession()
		if err != nil {
			return
		}
		return f.httpPatchJSON(url, data)
	}
	return
}

func (f *Force) httpPatchWithContentType(url string, data string, contenttype string) (body []byte, err error) {
	body, err = f.httpPostPatchWithContentType(url, data, contenttype, "PATCH")
	return
}

func (f *Force) httpPostWithContentType(url string, data string, contenttype string, requestOptions ...func(*http.Request)) (body []byte, err error) {
	body, err = f.httpPostPatchWithContentType(url, data, contenttype, "POST", requestOptions...)
	return
}

func (f *Force) httpPostPatchWithContentType(url string, data string, contenttype string, method string, requestOptions ...func(*http.Request)) (body []byte, err error) {
	rbody := data
	req, err := httpRequest(strings.ToUpper(method), url, bytes.NewReader([]byte(rbody)))
	if err != nil {
		return
	}

	for _, option := range requestOptions {
		option(req)
	}

	req.Header.Add("X-SFDC-Session", f.Credentials.AccessToken)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", f.Credentials.AccessToken))
	req.Header.Add("Content-Type", contenttype)
	res, err := doRequest(req)
	if err != nil {
		return
	}
	defer res.Body.Close()
	if res.StatusCode == 401 {
		err = SessionExpiredError
		return
	}
	body, err = ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}
	if res.StatusCode/100 != 2 {
		if contenttype == "application/xml" {
			var fault LoginFault
			xml.Unmarshal(body, &fault)
			if fault.ExceptionCode == "InvalidSessionId" {
				err = SessionExpiredError
			}
		} else {
			var messages []ForceError
			json.Unmarshal(body, &messages)
			if messages != nil {
				err = errors.New(messages[0].Message)
			}
		}
		return
	} else if res.StatusCode == 204 {
		body = []byte("Patch command succeeded....")
	}
	return
}

func (f *Force) httpPost(url string, attrs map[string]string) (body []byte, err error, emessages []ForceError) {
	body, err, emessages = f.httpPostAttributes(url, attrs)
	if err == SessionExpiredError {
		err = f.RefreshSession()
		if err != nil {
			return
		}
		return f.httpPost(url, attrs)
	}
	return
}

func (f *Force) httpPostAttributes(url string, attrs map[string]string) (body []byte, err error, emessages []ForceError) {
	rbody, _ := json.Marshal(attrs)
	req, err := httpRequest("POST", url, bytes.NewReader(rbody))
	if err != nil {
		return
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", f.Credentials.AccessToken))
	req.Header.Add("Content-Type", "application/json")
	res, err := doRequest(req)
	if err != nil {
		return
	}
	defer res.Body.Close()
	if res.StatusCode == 401 {
		err = SessionExpiredError
		return
	}
	body, err = ioutil.ReadAll(res.Body)
	if res.StatusCode/100 != 2 {
		var messages []ForceError
		json.Unmarshal(body, &messages)
		err = errors.New(messages[0].Message)
		emessages = messages
		return
	}
	return
}

func (f *Force) httpPatch(url string, attrs map[string]string) (body []byte, err error) {
	body, err = f.httpPatchAttributes(url, attrs)
	if err == SessionExpiredError {
		err = f.RefreshSession()
		if err != nil {
			return
		}
		return f.httpPatchAttributes(url, attrs)
	}
	return
}

func (f *Force) httpPatchAttributes(url string, attrs map[string]string) (body []byte, err error) {
	rbody, _ := json.Marshal(attrs)
	req, err := httpRequest("PATCH", url, bytes.NewReader(rbody))
	if err != nil {
		return
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", f.Credentials.AccessToken))
	req.Header.Add("Content-Type", "application/json")
	res, err := doRequest(req)
	if err != nil {
		return
	}
	defer res.Body.Close()
	if res.StatusCode == 401 {
		err = SessionExpiredError
		return
	}
	body, err = ioutil.ReadAll(res.Body)
	if res.StatusCode/100 != 2 {
		var messages []ForceError
		json.Unmarshal(body, &messages)
		err = errors.New(messages[0].Message)
		return
	}
	return
}

func (f *Force) httpDelete(url string) (body []byte, err error) {
	body, err = f.httpDeleteUrl(url)
	if err == SessionExpiredError {
		err = f.RefreshSession()
		if err != nil {
			return
		}
		return f.httpDeleteUrl(url)
	}
	return
}

func (f *Force) httpDeleteUrl(url string) (body []byte, err error) {
	req, err := httpRequest("DELETE", url, nil)
	if err != nil {
		return
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", f.Credentials.AccessToken))
	res, err := doRequest(req)
	if err != nil {
		return
	}
	defer res.Body.Close()
	if res.StatusCode == 401 {
		err = SessionExpiredError
		return
	}
	body, err = ioutil.ReadAll(res.Body)
	if res.StatusCode/100 != 2 {
		var messages []ForceError
		json.Unmarshal(body, &messages)
		err = errors.New(messages[0].Message)
		return
	}
	return
}

func (result *ForceQueryResult) Update(other ForceQueryResult, force *Force) {
	result.Done = other.Done
	result.Records = append(result.Records, other.Records...)
	result.TotalSize = len(result.Records)
	result.NextRecordsUrl = fmt.Sprintf("%s%s", force.Credentials.InstanceUrl, other.NextRecordsUrl)
}

func doRequest(request *http.Request) (res *http.Response, err error) {
	client := &http.Client{}
	client.Timeout = time.Duration(Timeout) * time.Millisecond
	return client.Do(request)
}

func httpRequest(method, url string, body io.Reader) (request *http.Request, err error) {
	request, err = http.NewRequest(method, url, body)
	if err != nil {
		return
	}
	request.Header.Add("User-Agent", fmt.Sprintf("force/%s (%s-%s)", Version, runtime.GOOS, runtime.GOARCH))
	return
}

func startLocalHttpServer(ch chan ForceSession) (port int, err error) {
	listener, err := net.Listen("tcp", ":3835")
	if err != nil {
		return
	}
	port = listener.Addr().(*net.TCPAddr).Port
	h := http.NewServeMux()
	h.HandleFunc("/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.Method == "POST" {
			var creds ForceSession
			creds.AccessToken = query.Get("access_token")
			creds.RefreshToken = query.Get("refresh_token")
			creds.InstanceUrl = query.Get("instance_url")
			creds.IssuedAt = query.Get("issued_at")
			creds.Scope = query.Get("scope")
			ch <- creds
			listener.Close()
		} else {
			io.WriteString(w, oauthCallbackHtml())
		}
	})
	go http.Serve(listener, h)
	return
}

func oauthCallbackHtml() string {
	return `
<!doctype html>
<html>
  <head>
	  <title>Force CLI OAuth Callback</title>
  </head>
  <body>
	  <h1>OAuth Callback</h1>
	  <p id="status">Status: Idle</p>
	  <script type="text/javascript">
	  window.onload = function() {
		  var url = window.location.href.replace('#', '?');
		  var req = new XMLHttpRequest();
		  var completeText = 'Complete! You may now close this window';
		  var errorText = 'Something went wrong!';
		  var statusEl = document.getElementById('status');

		  req.onreadystatechange=function() {

			  if(req.readyState==4 && req.status==200) {
				  statusEl.innerHTML = completeText;
			  } else {
				  statusEl.innerHTML = errorText;
			  }
		  }

		  req.open('POST', url, true);
		  req.setRequestHeader('Content-Type', 'text/plain');
		  statusEl.innerHTML = 'Status: Sending Auth...';
		  req.send();
	  }
	  </script>
  </body>
</html>`
}

func (force *Force) GetMetadata() *ForceMetadata {
	return force.Metadata
}

func (force *Force) GetCredentials() *ForceSession {
	return force.Credentials
}

func (force *Force) GetPartner() *ForcePartner {
	return force.Partner
}
