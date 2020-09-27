package servicerecord

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	internalapi "k8s.io/cri-api/pkg/apis"
	"k8s.io/kubernetes/pkg/kubelet/remote"
)

const (
	configPath          = "/etc/kubernetes/runtimeList.conf"
	jsonPath            = "/etc/kubernetes/tmpResult.json"
	experientRuntimeNum = 3
)

type GrpcResult struct {
	Runtime internalapi.RuntimeService
	Image   internalapi.ImageManagerService
}

type SockRecord struct {
	Sock       []string
	Service    map[string]GrpcResult
	ActiveTime map[string]time.Time
	Period     map[string]metav1.Duration
}

func NewSockRecord() (result SockRecord) {
	result = SockRecord{
		Sock:       make([]string, 0, experientRuntimeNum),
		Service:    make(map[string]GrpcResult),
		ActiveTime: make(map[string]time.Time),
		Period:     make(map[string]metav1.Duration),
	}
	_ = result.LoadConfig()
	return result
}

func (s *SockRecord) LoadConfig() error {
	file, err := ioutil.ReadFile(configPath)
	if err != nil {
		return err
	}
	err = json.Unmarshal([]byte(file), &s.Sock)
	if err != nil {
		return err
	}
	return nil
}

func (s *SockRecord) SaveService() error {
	file, err := json.MarshalIndent(*s, "", " ")
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(jsonPath, file, 0644)
	if err != nil {
		return err
	}
	return nil
}

func (s *SockRecord) Add(sock string,
	runtime internalapi.RuntimeService,
	image internalapi.ImageManagerService,
	t time.Time,
	period metav1.Duration) {
	result := GrpcResult{
		Runtime: runtime,
		Image:   image,
	}
	s.Sock = append(s.Sock, sock)
	s.Service[sock] = result
	s.ActiveTime[sock] = t
	s.Period[sock] = period
}

/*
*	In the future,this func will replace func getRuntimeAndImageServices in option pakcage
*	check whether question can be satified by existed service list
 */
func (s *SockRecord) IsExistedService(remoteRuntimeEndpoint string,
	remoteImageEndpoint string,
	runtimeRequestTimeout metav1.Duration) (internalapi.RuntimeService, internalapi.ImageManagerService, error) {
	//For now,we
	ok := isSameSock(remoteRuntimeEndpoint, remoteImageEndpoint)
	if !ok {
		return nil, nil, errors.New("Failure:Sockendpoints is different.")
	}

	//Existed sock searching
	requestSock := remoteRuntimeEndpoint
	existed := false
	var sock string
	for _, sock = range s.Sock {
		if sock == requestSock {
			existed = true
			break
		}
	}

	//Unkown service request
	if !existed {
		rs, err := remote.NewRemoteRuntimeService(requestSock, runtimeRequestTimeout.Duration)
		if err != nil {
			return nil, nil, err
		}
		is, err := remote.NewRemoteImageService(requestSock, runtimeRequestTimeout.Duration)
		if err != nil {
			return nil, nil, err
		}
		t := time.Now()
		s.Add(requestSock, rs, is, t, runtimeRequestTimeout)
		return rs, is, err
	}

	//Timeout check
	active := s.ActiveTime[requestSock]
	period := s.Period[requestSock]
	ok = isLegelService(period, runtimeRequestTimeout, active)
	if !ok {
		return nil, nil, errors.New("Failure:Existed services don't satify requestion.")
	}

	service := s.Service[requestSock]
	return service.Runtime, service.Image, nil
}

//private
func isLegelService(period, request metav1.Duration, active time.Time) bool {
	except := time.Now().Add(request.Duration)
	deadline := active.Add(period.Duration)
	diff := deadline.Sub(except)
	if diff >= 0 {
		return true
	}
	return false
}

func isSameSock(runtimeEndpoint string, imageEndpoint string) bool {
	if runtimeEndpoint == imageEndpoint {
		return true
	}
	return false
}

