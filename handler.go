package gows

type Handler interface {
	/**
	 * 客户端连接成功触发
	 * 建议场景：（1）做校验，例如token失效，那么主动断开连接；（2）做日志记录
	 * conn：连接
	 * urlParameterMap：websocket请求地址附带的参数
	 * headerMap：请求头数据
	 */

	Connected(conn *ConnectionWrap, urlParameterMap map[string]string, headerMap map[string]string)

	/**
	 * 客户端发送数据进来触发
	 * 建议场景：（1）业务处理；（2）做日志记录
	 * conn：连接
	 * data：业务数据
	 * urlParameterMap：websocket请求地址附带的参数
	 * headerMap：请求头数据
	 */

	Do(conn *ConnectionWrap, data Data, urlParameterMap map[string]string, headerMap map[string]string)

	/**
	 * 客户端断开连接触发
	 * 建议场景：（1）做日志记录；（2）不需要再手工断开连接
	 * conn：连接
	 * urlParameterMap：websocket请求地址附带的参数
	 * headerMap：请求头数据
	 */

	Disconnected(conn *ConnectionWrap, urlParameterMap map[string]string, headerMap map[string]string)
}

type Data struct {
	/**
	 * 数据类型
	 * TextMessage = 1
	 * BinaryMessage = 2
	 * CloseMessage = 8
	 * PingMessage = 9
	 * PongMessage = 10
	 */
	MessageType int

	/**
	 * 数据内容
	 */
	MessageData []byte
}
