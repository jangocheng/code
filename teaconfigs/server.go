package teaconfigs

import (
	"errors"
	"github.com/TeaWeb/code/teaconfigs/api"
	"github.com/TeaWeb/code/teaconfigs/scheduling"
	"github.com/TeaWeb/code/teaconfigs/shared"
	"github.com/iwind/TeaGo/Tea"
	"github.com/iwind/TeaGo/files"
	"github.com/iwind/TeaGo/lists"
	"github.com/iwind/TeaGo/logs"
	"github.com/iwind/TeaGo/maps"
	"github.com/iwind/TeaGo/utils/string"
	"github.com/mozillazg/go-pinyin"
	"strings"
	"sync"
)

// 服务配置
type ServerConfig struct {
	shared.HeaderList `yaml:",inline"`
	FastcgiList       `yaml:",inline"`
	RewriteList       `yaml:",inline"`

	On bool `yaml:"on" json:"on"` // 是否开启 @TODO

	Id          string   `yaml:"id" json:"id"`                   // ID
	Description string   `yaml:"description" json:"description"` // 描述
	Name        []string `yaml:"name" json:"name"`               // 域名
	Http        bool     `yaml:"http" json:"http"`               // 是否支持HTTP

	// 监听地址
	// @TODO 支持参数，比如：127.0.01:1234?ssl=off
	Listen []string `yaml:"listen" json:"listen"`

	Root       string                 `yaml:"root" json:"root"`             // 资源根目录 @TODO
	Index      []string               `yaml:"index" json:"index"`           // 默认文件 @TODO
	Charset    string                 `yaml:"charset" json:"charset"`       // 字符集 @TODO
	Backends   []*ServerBackendConfig `yaml:"backends" json:"backends"`     // 后端服务器配置
	Scheduling *SchedulingConfig      `yaml:"scheduling" json:"scheduling"` // 调度算法选项
	Locations  []*LocationConfig      `yaml:"locations" json:"locations"`   // 地址配置

	Async   bool     `yaml:"async" json:"async"`     // 请求是否异步处理 @TODO
	Notify  []string `yaml:"notify" json:"notify"`   // 请求转发地址 @TODO
	LogOnly bool     `yaml:"logOnly" json:"logOnly"` // 是否只记录日志 @TODO

	// 访问日志
	AccessLog []*AccessLogConfig `yaml:"accessLog" json:"accessLog"` // 访问日志

	// @TODO 支持ErrorLog

	// SSL
	SSL *SSLConfig `yaml:"ssl" json:"ssl"`

	// 参考：http://nginx.org/en/docs/http/ngx_http_access_module.html
	Allow []string `yaml:"allow" json:"allow"` //TODO
	Deny  []string `yaml:"deny" json:"deny"`   //TODO

	Filename string `yaml:"filename" json:"filename"` // 配置文件名

	Proxy string `yaml:"proxy" json:"proxy"` //  代理配置 TODO

	CachePolicy string `yaml:"cachePolicy" json:"cachePolicy"` // 缓存策略
	CacheOn     bool   `yaml:"cacheOn" json:"cacheOn"`         // 缓存是否打开 TODO
	cachePolicy *shared.CachePolicy

	// API相关
	API *api.APIConfig `yaml:"api" json:"api"` // API配置

	schedulingIsBackup bool
	schedulingObject   scheduling.SchedulingInterface
	schedulingLocker   sync.Mutex
}

// 从目录中加载配置
func LoadServerConfigsFromDir(dirPath string) []*ServerConfig {
	servers := []*ServerConfig{}

	dir := files.NewFile(dirPath)
	subFiles := dir.Glob("*.proxy.conf")
	for _, configFile := range subFiles {
		reader, err := configFile.Reader()
		if err != nil {
			logs.Error(err)
			continue
		}

		config := &ServerConfig{}
		err = reader.ReadYAML(config)
		if err != nil {
			continue
		}
		config.Filename = configFile.Name()

		// API
		if config.API == nil {
			config.API = api.NewAPIConfig()
		}

		servers = append(servers, config)
	}

	lists.Sort(servers, func(i int, j int) bool {
		s1 := servers[i].convertPinYin(servers[i].Description)
		s2 := servers[j].convertPinYin(servers[j].Description)
		return strings.Compare(s1, s2) < 0
	})

	return servers
}

// 取得一个新的服务配置
func NewServerConfig() *ServerConfig {
	return &ServerConfig{
		On:  true,
		Id:  stringutil.Rand(16),
		API: api.NewAPIConfig(),
	}
}

// 从配置文件中读取配置信息
func NewServerConfigFromFile(filename string) (*ServerConfig, error) {
	if len(filename) == 0 {
		return nil, errors.New("filename should not be empty")
	}
	reader, err := files.NewReader(Tea.ConfigFile(filename))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	config := &ServerConfig{}
	err = reader.ReadYAML(config)
	if err != nil {
		return nil, err
	}

	config.Filename = filename

	// 初始化
	if len(config.Locations) == 0 {
		config.Locations = []*LocationConfig{}
	}
	if config.API == nil {
		config.API = api.NewAPIConfig()
	}
	if config.Headers == nil {
		config.Headers = []*shared.HeaderConfig{}
	}
	if config.IgnoreHeaders == nil {
		config.IgnoreHeaders = []string{}
	}

	return config, nil
}

// 校验配置
func (this *ServerConfig) Validate() error {
	// ssl
	if this.SSL != nil {
		err := this.SSL.Validate()
		if err != nil {
			return err
		}
	}

	// backends
	for _, backend := range this.Backends {
		err := backend.Validate()
		if err != nil {
			return err
		}
	}

	// scheduling
	this.SetupScheduling(false)

	// locations
	for _, location := range this.Locations {
		err := location.Validate()
		if err != nil {
			return err
		}
	}

	// fastcgi
	err := this.ValidateFastcgi()
	if err != nil {
		return err
	}

	// rewrite rules
	err = this.ValidateRewriteRules()
	if err != nil {
		return err
	}

	// headers
	err = this.ValidateHeaders()
	if err != nil {
		return err
	}

	// 校验缓存配置
	if len(this.CachePolicy) > 0 {
		policy := shared.NewCachePolicyFromFile(this.CachePolicy)
		if policy != nil {
			err := policy.Validate()
			if err != nil {
				return err
			}
			this.cachePolicy = policy
		}
	}

	// api
	if this.API == nil {
		this.API = api.NewAPIConfig()
	}

	err = this.API.Validate()
	if err != nil {
		return err
	}

	return nil
}

// 添加域名
func (this *ServerConfig) AddName(name ... string) {
	this.Name = append(this.Name, name ...)
}

// 添加监听地址
func (this *ServerConfig) AddListen(address string) {
	this.Listen = append(this.Listen, address)
}

// 添加后端服务
func (this *ServerConfig) AddBackend(config *ServerBackendConfig) {
	this.Backends = append(this.Backends, config)
}

// 取得下一个可用的后端服务
func (this *ServerConfig) NextBackend(options maps.Map) *ServerBackendConfig {
	this.schedulingLocker.Lock()
	defer this.schedulingLocker.Unlock()

	if this.schedulingObject == nil {
		return nil
	}

	if this.Scheduling != nil {
		for k, v := range this.Scheduling.Options {
			options[k] = v
		}
	}

	candidate := this.schedulingObject.Next(options)
	if candidate == nil {
		// 启用备用服务器
		if !this.schedulingIsBackup {
			this.SetupScheduling(true)

			candidate = this.schedulingObject.Next(options)
			if candidate == nil {
				return nil
			}
		}

		if candidate == nil {
			return nil
		}
	}

	return candidate.(*ServerBackendConfig)
}

// 根据ID查找后端服务器
func (this *ServerConfig) FindBackend(backendId string) *ServerBackendConfig {
	for _, backend := range this.Backends {
		if backend.Id == backendId {
			return backend
		}
	}
	return nil
}

// 删除后端服务器
func (this *ServerConfig) DeleteBackend(backendId string) {
	result := []*ServerBackendConfig{}
	for _, backend := range this.Backends {
		if backend.Id == backendId {
			continue
		}
		result = append(result, backend)
	}
	this.Backends = result
}

// 获取某个位置上的配置
func (this *ServerConfig) LocationAtIndex(index int) *LocationConfig {
	if index < 0 {
		return nil
	}
	if index >= len(this.Locations) {
		return nil
	}
	location := this.Locations[index]
	location.Validate()
	return location
}

// 将配置写入文件
func (this *ServerConfig) WriteToFile(path string) error {
	writer, err := files.NewWriter(path)
	if err != nil {
		return err
	}
	_, err = writer.WriteYAML(this)
	writer.Close()
	return err
}

// 将配置写入文件
func (this *ServerConfig) WriteToFilename(filename string) error {
	writer, err := files.NewWriter(Tea.ConfigFile(filename))
	if err != nil {
		return err
	}
	_, err = writer.WriteYAML(this)
	writer.Close()
	return err
}

// 保存
func (this *ServerConfig) Save() error {
	if len(this.Filename) == 0 {
		return errors.New("'filename' should be specified")
	}

	return this.WriteToFilename(this.Filename)
}

// 判断是否和域名匹配
// @TODO 支持  .example.com （所有以example.com结尾的域名，包括example.com）
// 更多参考：http://nginx.org/en/docs/http/ngx_http_core_module.html#server_name
func (this *ServerConfig) MatchName(name string) (matchedName string, matched bool) {
	if len(name) == 0 {
		return "", false
	}
	pieces1 := strings.Split(name, ".")
	countPieces1 := len(pieces1)
	for _, testName := range this.Name {
		if len(testName) == 0 {
			continue
		}
		if name == testName {
			return testName, true
		}
		pieces2 := strings.Split(testName, ".")
		if countPieces1 != len(pieces2) {
			continue
		}
		matched := true
		for index, piece := range pieces2 {
			if pieces1[index] != piece && piece != "*" && piece != "" {
				matched = false
				break
			}
		}
		if matched {
			return "", true
		}
	}
	return "", false
}

// 取得第一个非泛解析的域名
func (this *ServerConfig) FirstName() string {
	for _, name := range this.Name {
		if strings.Contains(name, "*") {
			continue
		}
		return name
	}
	return ""
}

// 添加路径规则
func (this *ServerConfig) AddLocation(location *LocationConfig) {
	this.Locations = append(this.Locations, location)
}

// 缓存策略
func (this *ServerConfig) CachePolicyObject() *shared.CachePolicy {
	return this.cachePolicy
}

// 拼音转换，用户转换代理描述中的中文
func (this *ServerConfig) convertPinYin(s string) string {
	a := pinyin.NewArgs()

	result := []string{}
	for _, rune1 := range []rune(s) {
		r := string(rune1)
		if len(r) == 1 {
			result = append(result, r)
		} else {
			for _, s := range pinyin.Convert(r, &a) {
				if len(s) > 0 {
					result = append(result, s[0]+" ")
				}
			}
		}
	}
	return strings.Join(result, "")
}

// 设置调度算法
func (this *ServerConfig) SetupScheduling(isBackup bool) {
	if !isBackup {
		this.schedulingLocker.Lock()
		defer this.schedulingLocker.Unlock()
	}
	this.schedulingIsBackup = isBackup

	if this.Scheduling == nil {
		this.schedulingObject = &scheduling.RandomScheduling{}
	} else {
		typeCode := this.Scheduling.Code
		s := scheduling.FindSchedulingType(typeCode)
		if s == nil {
			this.Scheduling = nil
			this.schedulingObject = &scheduling.RandomScheduling{}
		} else {
			this.schedulingObject = s["instance"].(scheduling.SchedulingInterface)
		}
	}

	for _, backend := range this.Backends {
		if backend.On && !backend.IsDown {
			if isBackup && backend.IsBackup {
				this.schedulingObject.Add(backend)
			} else if !isBackup && !backend.IsBackup {
				this.schedulingObject.Add(backend)
			}
		}
	}

	this.schedulingObject.Start()
}

// 根据Id查找Location
func (this *ServerConfig) FindLocation(locationId string) *LocationConfig {
	for _, location := range this.Locations {
		if location.Id == locationId {
			location.Validate()
			return location
		}
	}
	return nil
}

// 删除Location
func (this *ServerConfig) RemoveLocation(locationId string) {
	result := []*LocationConfig{}
	for _, location := range this.Locations {
		if location.Id == locationId {
			continue
		}
		result = append(result, location)
	}
	this.Locations = result
}

// 查找HeaderList
func (this *ServerConfig) FindHeaderList(locationId string, backendId string, rewriteId string, fastcgiId string) (headerList shared.HeaderListInterface, err error) {
	if len(rewriteId) > 0 { // Rewrite
		if len(locationId) > 0 { // Server > Location > Rewrite
			location := this.FindLocation(locationId)
			if location == nil {
				err = errors.New("找不到要修改的location")
				return
			}

			rewrite := location.FindRewriteRule(rewriteId)
			if rewrite == nil {
				err = errors.New("找不到要修改的rewrite")
				return
			}
			headerList = rewrite
		} else { // Server > Rewrite
			rewrite := this.FindRewriteRule(rewriteId)
			if rewrite == nil {
				err = errors.New("找不到要修改的rewrite")
				return
			}
			headerList = rewrite
		}
	} else if len(fastcgiId) > 0 { // Fastcgi
		if len(locationId) > 0 { // Server > Location > Fastcgi
			location := this.FindLocation(locationId)
			if location == nil {
				err = errors.New("找不到要修改的location")
				return
			}

			fastcgi := location.FindFastcgi(fastcgiId)
			if fastcgi == nil {
				err = errors.New("找不到要修改的Fastcgi")
				return
			}
			headerList = fastcgi
		} else { // Server > Fastcgi
			fastcgi := this.FindFastcgi(fastcgiId)
			if fastcgi == nil {
				err = errors.New("找不到要修改的Fastcgi")
				return
			}
			headerList = fastcgi
		}
	} else if len(backendId) > 0 { // Backend
		if len(locationId) > 0 { // Server > Location > Backend
			location := this.FindLocation(locationId)
			if location == nil {
				err = errors.New("找不到要修改的location")
				return
			}

			backend := location.FindBackend(backendId)
			if backend == nil {
				err = errors.New("找不到要修改的Backend")
				return
			}
			headerList = backend
		} else { // Server > Backend
			backend := this.FindBackend(backendId)
			if backend == nil {
				err = errors.New("找不到要修改的Backend")
				return
			}
			headerList = backend
		}
	} else if len(locationId) > 0 { // Location
		location := this.FindLocation(locationId)
		if location == nil {
			err = errors.New("找不到要修改的location")
			return
		}
		headerList = location
	} else { // Server
		headerList = this
	}

	return
}

// 查找FastcgiList
func (this *ServerConfig) FindFastcgiList(locationId string) (fastcgiList FastcgiListInterface, err error) {
	if len(locationId) > 0 {
		location := this.FindLocation(locationId)
		if location == nil {
			err = errors.New("找不到要修改的location")
			return
		}
		fastcgiList = location
		return
	}
	fastcgiList = this
	return
}

// 查找重写规则
func (this *ServerConfig) FindRewriteList(locationId string) (rewriteList RewriteListInterface, err error) {
	if len(locationId) > 0 {
		location := this.FindLocation(locationId)
		if location == nil {
			err = errors.New("找不到要修改的location")
			return
		}
		rewriteList = location
		return
	}
	rewriteList = this
	return
}
