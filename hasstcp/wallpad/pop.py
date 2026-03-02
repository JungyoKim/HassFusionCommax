import requests

# 요청 본문 (XML 데이터)
# popVList는 일반적으로 특정 인자를 받지 않거나,
# 어떤 항목을 pop할지 식별하는 인자를 받을 수 있습니다.
# 여기서는 가장 단순한 호출 형태로 가정합니다.
xml_payload_pop = """<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/" xmlns:SOAP-ENC="http://schemas.xmlsoap.org/soap/encoding/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:ns1="urn:cmm">
    <SOAP-ENV:Body SOAP-ENV:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
        <ns1:popVList>
            </ns1:popVList>
    </SOAP-ENV:Body>
</SOAP-ENV:Envelope>"""

# 요청 헤더 (pushVList와 동일한 호스트 및 기타 정보 사용)
headers_pop = {
    "Host": "10.9.1.27:29720", # 이전 요청과 동일한 호스트 사용
    "User-Agent": "gSOAP/2.7",
    "Content-Type": "text/xml; charset=utf-8",
    "Content-Length": str(len(xml_payload_pop.encode('utf-8'))), # UTF-8 인코딩 후 길이 계산
    "Connection": "close",
    "SOAPAction": '""' # 빈 문자열 SOAPAction (일반적으로 SOAPAction은 메서드 이름을 포함할 수도 있습니다.)
}

# 요청 URL
url_pop = "http://10.9.1.27:29720/" # 이전 요청과 동일한 URL 사용

print("--- popVList SOAP 요청 보내기 ---")

try:
    # POST 요청 보내기
    response_pop = requests.post(url_pop, headers=headers_pop, data=xml_payload_pop, verify=False) # SSL/TLS 검증 비활성화 (테스트 목적)

    # 응답 출력
    print("--- 서버 응답 ---")
    print(f"상태 코드: {response_pop.status_code}")
    print("응답 헤더:")
    for key, value in response_pop.headers.items():
        print(f"  {key}: {value}")
    print("응답 본문:")
    print(response_pop.text)

except requests.exceptions.RequestException as e:
    print(f"요청 중 오류 발생: {e}")