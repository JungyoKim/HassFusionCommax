import requests

# 요청 본문 (XML 데이터)
xml_payload = """<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/" xmlns:SOAP-ENC="http://schemas/xmlsoap/encoding/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:ns1="urn:cmm">
    <SOAP-ENV:Body SOAP-ENV:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
        <ns1:pushVList>
            <ip>10.9.12.11</ip>
            <port>60006</port>
        </ns1:pushVList>
    </SOAP-ENV:Body>
</SOAP-ENV:Envelope>"""

# 요청 헤더
headers = {
    "Host": "10.9.1.37:29720",
    "User-Agent": "gSOAP/2.7",
    "Content-Type": "text/xml; charset=utf-8",
    "Content-Length": str(len(xml_payload.encode('utf-8'))), # UTF-8 인코딩 후 길이 계산
    "Connection": "close",
    "SOAPAction": '""' # 빈 문자열 SOAPAction
}

# 요청 URL
url = "http://10.9.1.27:29720/" # 호스트와 포트를 URL에 포함

try:
    # POST 요청 보내기
    response = requests.post(url, headers=headers, data=xml_payload, verify=False) # SSL/TLS 검증 비활성화 (필요한 경우)

    # 응답 출력
    print("--- 응답 ---")
    print(f"상태 코드: {response.status_code}")
    print("응답 헤더:")
    for key, value in response.headers.items():
        print(f"  {key}: {value}")
    print("응답 본문:")
    print(response.text)

except requests.exceptions.RequestException as e:
    print(f"요청 중 오류 발생: {e}")