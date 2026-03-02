# import requests
# import datetime

# # 요청을 보낼 URL
# # 이 요청은 10.9.12.11:29707로 보내졌으므로, 실제 서비스가 이 주소에 있다고 가정합니다.
# # 만약 OpenWrt 내부의 월패드나 다른 장치가 응답하는 것이라면,
# # OpenWrt에서 이 포트를 해당 장치로 포워딩하고 있을 것입니다.
# # 여기서는 캡처된대로 OpenWrt WAN IP를 대상으로 합니다.
# url = "http://192.168.1.7:29707/"

# # HTTP 헤더
# headers = {
#     "Host": "192.168.1.7:29707",
#     "Connection": "Keep-Alive",
#     "User-Agent": "PHP-SOAP/5.2.6", # 원본 요청의 User-Agent를 유지
#     "Content-Type": "text/xml; charset=utf-8",
#     "SOAPAction": "", # 원본 요청과 동일하게 비워둠
#     # Content-Length는 requests 라이브러리가 자동으로 계산해줍니다.
# }

# # SOAP 요청 바디 (XML)
# # 시간과 차량 번호는 동적으로 변경할 수 있습니다.
# current_time = datetime.datetime.now().strftime("%Y-%m-%dT%H:%M:%S+09:00")
# car_number = "테스트입니다" # 예시 차량 번호, 필요에 따라 변경

# xml_body = f"""<?xml version="1.0" encoding="UTF-8"?>
# <SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/" xmlns:ns1="urn:cls" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:SOAP-ENC="http://schemas.xmlsoap.org/soap/encoding/" SOAP-ENV:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
#   <SOAP-ENV:Body>
#     <ns1:parkService>
#       <in xsi:type="ns1:park">
#         <time xsi:type="xsd:dateTime">{current_time}</time>
#         <type xsi:type="ns1:enum-ls">parkIn</type>
#         <carNo xsi:type="xsd:string">{car_number}</carNo>
#       </in>
#     </ns1:parkService>
#   </SOAP-ENV:Body>
# </SOAP-ENV:Envelope>"""

# try:
#     # POST 요청 보내기
#     response = requests.post(url, headers=headers, data=xml_body)

#     # 응답 확인
#     print(f"Status Code: {response.status_code}")
#     print("Response Headers:")
#     for header, value in response.headers.items():
#         print(f"  {header}: {value}")
#     print("\nResponse Body:")
#     print(response.text)

#     # 응답이 성공적이고, 내용에 <out>0</out>이 있는지 확인
#     if response.status_code == 200 and "<out>0</out>" in response.text:
#         print("\n요청 성공: 'parkIn' 서비스가 정상적으로 처리된 것으로 보입니다.")
#     else:
#         print("\n요청 실패 또는 예상치 못한 응답입니다.")

# except requests.exceptions.RequestException as e:
#     print(f"요청 중 오류 발생: {e}")


import requests
import datetime

# 요청을 보낼 URL
url = "http://192.168.1.7:29707/"

# HTTP 헤더
headers = {
    "Host": "192.168.1.7:29707",
    "Connection": "Keep-Alive",
    "User-Agent": "PHP-SOAP/5.2.6",
    "Content-Type": "text/xml; charset=utf-8",
    "SOAPAction": "",
}

# SOAP 요청 바디 (XML)
current_time = datetime.datetime.now().strftime("%Y-%m-%dT%H:%M:%S+09:00")
# car_number = "testtesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttes"

car_number_options = [
    "███████████████████████████████████████████████████████████████████",           # 꽉 찬 네모
    "■12가3456■",        # 검은 네모
    "▆ABC123▆",          # 블록 문자
    "⬛VIP⬛",            # 큰 검은 네모
    "◆다이아◆",          # 다이아몬드
    "★스타★",           # 별
    "♦클럽♦",           # 다이아몬드 슈트
    "▲삼각▲",           # 삼각형
    "●원형●",           # 검은 원
    "◎타겟◎",           # 타겟
    "※특수※",           # 별표
    "♠스페이드♠",        # 스페이드
    "◈다이아◈",          # 흰 다이아몬드
    "학원간데요~ㅋㅋ",           # 체크 박스
]

# 원하는 옵션을 선택하세요 (0-13)
car_number = car_number_options[13]  # 기본값: █테스트█
xml_body = f"""<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/" xmlns:ns1="urn:cls" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:SOAP-ENC="http://schemas.xmlsoap.org/soap/encoding/" SOAP-ENV:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <SOAP-ENV:Body>
    <ns1:parkService>
      <in xsi:type="ns1:park">
        <time xsi:type="xsd:dateTime">{current_time}</time>
        <type xsi:type="ns1:enum-ls">parkOut</type>
        <carNo xsi:type="xsd:string">{car_number}</carNo>
      </in>
    </ns1:parkService>
  </SOAP-ENV:Body>
</SOAP-ENV:Envelope>"""

try:
    # POST 요청 보내기 - data 대신 xml_body.encode('utf-8') 사용
    response = requests.post(url, headers=headers, data=xml_body.encode('utf-8'))

    # 응답 확인
    print(f"Status Code: {response.status_code}")
    print("Response Headers:")
    for header, value in response.headers.items():
        print(f"  {header}: {value}")
    print("\nResponse Body:")
    print(response.text)

    # 응답이 성공적이고, 내용에 <out>0</out>이 있는지 확인
    if response.status_code == 200 and "<out>0</out>" in response.text:
        print("\n요청 성공: 'parkIn' 서비스가 정상적으로 처리된 것으로 보입니다.")
    else:
        print("\n요청 실패 또는 예상치 못한 응답입니다.")

except requests.exceptions.RequestException as e:
    print(f"요청 중 오류 발생: {e}")
except UnicodeEncodeError as e:
    print(f"인코딩 오류: {e}")
except Exception as e:
    print(f"예상치 못한 오류: {e}")