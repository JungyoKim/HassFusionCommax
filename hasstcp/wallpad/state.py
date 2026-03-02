import requests
from lxml import etree # lxml 라이브러리 임포트

url = "http://10.0.0.2:29715"
headers = {
    "Content-Type": "text/xml; charset=utf-8",
    "SOAPAction": "urn:ces#getEvStatus" # 또는 "SOAPAction: \"\""
}
soap_body = """
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/" xmlns:SOAP-ENC="http://schemas.xmlsoap.org/soap/encoding/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:ns="urn:ces">
<SOAP-ENV:Body SOAP-ENV:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<ns:getEvStatus><in>1</in></ns:getEvStatus>
</SOAP-ENV:Body>
</SOAP-ENV:Envelope>
"""

try:
    response = requests.post(url, headers=headers, data=soap_body)
    response.raise_for_status()

    response_xml = response.text
    print("Raw XML Response:\n", response_xml)

    # lxml을 사용하여 XML 파싱
    root = etree.fromstring(response_xml.encode('utf-8'))

    # 네임스페이스 정의
    namespaces = {
        'SOAP-ENV': "http://schemas.xmlsoap.org/soap/envelope/",
        'ns': "urn:ces"
    }

    # 방법 1: 네임스페이스 없이 carFloor 찾기
    car_floors = root.xpath('.//carFloor')
    
    if car_floors:
        for i, floor_elem in enumerate(car_floors):
            print(f"엘리베이터 {i+1} 현재 층수: {floor_elem.text}")
    else:
        print("carFloor 정보를 찾을 수 없습니다.")
        
    # 방법 2: 더 상세한 정보 추출
    items = root.xpath('.//item')
    if items:
        print("\n=== 상세 엘리베이터 정보 ===")
        for i, item in enumerate(items):
            car_floor = item.find('carFloor')
            is_basement = item.find('isBasement')
            car_direction = item.find('carDirection')
            ev_status = item.find('evStatus')
            call_up = item.find('callUp')
            call_down = item.find('callDown')
            
            print(f"\n엘리베이터 {i+1}:")
            print(f"  현재 층수: {car_floor.text if car_floor is not None else 'N/A'}")
            print(f"  지하층 여부: {is_basement.text if is_basement is not None else 'N/A'}")
            print(f"  방향: {car_direction.text if car_direction is not None else 'N/A'} (0=정지, 1=상승, 2=하강)")
            print(f"  상태: {ev_status.text if ev_status is not None else 'N/A'}")
            print(f"  상행 호출: {call_up.text if call_up is not None else 'N/A'}")
            print(f"  하행 호출: {call_down.text if call_down is not None else 'N/A'}")

except requests.exceptions.RequestException as e:
    print(f"요청 실패: {e}")
except etree.XMLSyntaxError as e: # lxml의 파싱 오류 예외
    print(f"XML 파싱 실패: {e}")
except Exception as e:
    print(f"알 수 없는 오류 발생: {e}")