// Package compare 는 M2 대조·판정의 실제 로직 — 역할과 무관하다. auditor · Vendor · user 세 역할의
// core/compare/<역할> 은 각자의 tee/parse.Measurements 타입을 여기가 받는 모양(Items)으로 바꿔주기만
// 하는 얇은 델리게이트다. 로직을 고칠 땐 여기(compare.go · check.go) 한 곳만 고치면 된다.
//
// compare.go — 체인이 안 낀 비교 (파싱값 vs 빌드값 등). check.go — 체인 등록값이 한쪽에라도 끼면,
// 설계문서 M2 규칙대로 값 일치뿐 아니라 만료 · 승인 서명자(trusted)까지 본다.
package compare

// Item · Items — 이름 있는 측정값 하나, 그 묶음. 역할별 Measurement/Measurements 와 필드가 같다.
type Item struct {
	Name  string
	Value []byte
}

type Items []Item

func (items Items) Get(name string) []byte {
	for _, it := range items {
		if it.Name == name {
			return it.Value
		}
	}
	return nil
}

// Verdict — 판정 결과와 근거. Reason 이 비어 있으면 Pass 다.
type Verdict struct {
	Pass   bool
	Reason []string
}
