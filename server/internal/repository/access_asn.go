package repository

import "database/sql"

type AccessASN struct {
	ASN         uint64
	Name        string
	Country     string
	NetworkKind string
	Source      string
}

var builtinAccessASNMap = map[uint64]AccessASN{
	4134:   {ASN: 4134, Name: "China Telecom", Country: "CN", NetworkKind: "residential", Source: "builtin"},
	4809:   {ASN: 4809, Name: "China Telecom", Country: "CN", NetworkKind: "residential", Source: "builtin"},
	4812:   {ASN: 4812, Name: "China Telecom", Country: "CN", NetworkKind: "residential", Source: "builtin"},
	4816:   {ASN: 4816, Name: "China Telecom", Country: "CN", NetworkKind: "residential", Source: "builtin"},
	4837:   {ASN: 4837, Name: "China Unicom", Country: "CN", NetworkKind: "residential", Source: "builtin"},
	17621:  {ASN: 17621, Name: "China Unicom", Country: "CN", NetworkKind: "residential", Source: "builtin"},
	17622:  {ASN: 17622, Name: "China Unicom", Country: "CN", NetworkKind: "residential", Source: "builtin"},
	17623:  {ASN: 17623, Name: "China Unicom", Country: "CN", NetworkKind: "residential", Source: "builtin"},
	9808:   {ASN: 9808, Name: "China Mobile", Country: "CN", NetworkKind: "mobile", Source: "builtin"},
	56040:  {ASN: 56040, Name: "China Mobile", Country: "CN", NetworkKind: "mobile", Source: "builtin"},
	56041:  {ASN: 56041, Name: "China Mobile", Country: "CN", NetworkKind: "mobile", Source: "builtin"},
	56042:  {ASN: 56042, Name: "China Mobile", Country: "CN", NetworkKind: "mobile", Source: "builtin"},
	56044:  {ASN: 56044, Name: "China Mobile", Country: "CN", NetworkKind: "mobile", Source: "builtin"},
	56046:  {ASN: 56046, Name: "China Mobile", Country: "CN", NetworkKind: "mobile", Source: "builtin"},
	56047:  {ASN: 56047, Name: "China Mobile", Country: "CN", NetworkKind: "mobile", Source: "builtin"},
	56048:  {ASN: 56048, Name: "China Mobile", Country: "CN", NetworkKind: "mobile", Source: "builtin"},
	7640:   {ASN: 7640, Name: "China Broadnet", Country: "CN", NetworkKind: "residential", Source: "builtin"},
	4760:   {ASN: 4760, Name: "HKT", Country: "HK", NetworkKind: "residential", Source: "builtin"},
	9269:   {ASN: 9269, Name: "HKBN", Country: "HK", NetworkKind: "residential", Source: "builtin"},
	9304:   {ASN: 9304, Name: "HGC Global Communications", Country: "HK", NetworkKind: "residential", Source: "builtin"},
	4609:   {ASN: 4609, Name: "CTM", Country: "MO", NetworkKind: "residential", Source: "builtin"},
	3462:   {ASN: 3462, Name: "Chunghwa Telecom", Country: "TW", NetworkKind: "residential", Source: "builtin"},
	24158:  {ASN: 24158, Name: "Taiwan Mobile", Country: "TW", NetworkKind: "mobile", Source: "builtin"},
	9674:   {ASN: 9674, Name: "FarEasTone", Country: "TW", NetworkKind: "mobile", Source: "builtin"},
	9924:   {ASN: 9924, Name: "Taiwan Fixed Network", Country: "TW", NetworkKind: "residential", Source: "builtin"},
	7922:   {ASN: 7922, Name: "Comcast", Country: "US", NetworkKind: "residential", Source: "builtin"},
	7018:   {ASN: 7018, Name: "AT&T", Country: "US", NetworkKind: "residential", Source: "builtin"},
	20057:  {ASN: 20057, Name: "AT&T Mobility", Country: "US", NetworkKind: "mobile", Source: "builtin"},
	701:    {ASN: 701, Name: "Verizon", Country: "US", NetworkKind: "residential", Source: "builtin"},
	22394:  {ASN: 22394, Name: "Verizon Wireless", Country: "US", NetworkKind: "mobile", Source: "builtin"},
	20115:  {ASN: 20115, Name: "Charter Spectrum", Country: "US", NetworkKind: "residential", Source: "builtin"},
	10796:  {ASN: 10796, Name: "Charter Spectrum", Country: "US", NetworkKind: "residential", Source: "builtin"},
	11351:  {ASN: 11351, Name: "Charter Spectrum", Country: "US", NetworkKind: "residential", Source: "builtin"},
	11427:  {ASN: 11427, Name: "Charter Spectrum", Country: "US", NetworkKind: "residential", Source: "builtin"},
	21928:  {ASN: 21928, Name: "T-Mobile USA", Country: "US", NetworkKind: "mobile", Source: "builtin"},
	22773:  {ASN: 22773, Name: "Cox Communications", Country: "US", NetworkKind: "residential", Source: "builtin"},
	209:    {ASN: 209, Name: "CenturyLink", Country: "US", NetworkKind: "residential", Source: "builtin"},
	5650:   {ASN: 5650, Name: "Frontier Communications", Country: "US", NetworkKind: "residential", Source: "builtin"},
	6128:   {ASN: 6128, Name: "Optimum", Country: "US", NetworkKind: "residential", Source: "builtin"},
	16591:  {ASN: 16591, Name: "Google Fiber", Country: "US", NetworkKind: "residential", Source: "builtin"},
	14593:  {ASN: 14593, Name: "Starlink", Country: "US", NetworkKind: "residential", Source: "builtin"},
	577:    {ASN: 577, Name: "Bell Canada", Country: "CA", NetworkKind: "residential", Source: "builtin"},
	812:    {ASN: 812, Name: "Rogers Communications", Country: "CA", NetworkKind: "residential", Source: "builtin"},
	852:    {ASN: 852, Name: "TELUS", Country: "CA", NetworkKind: "residential", Source: "builtin"},
	6327:   {ASN: 6327, Name: "Shaw Communications", Country: "CA", NetworkKind: "residential", Source: "builtin"},
	5769:   {ASN: 5769, Name: "Videotron", Country: "CA", NetworkKind: "residential", Source: "builtin"},
	2856:   {ASN: 2856, Name: "BT", Country: "GB", NetworkKind: "residential", Source: "builtin"},
	5089:   {ASN: 5089, Name: "Virgin Media", Country: "GB", NetworkKind: "residential", Source: "builtin"},
	5607:   {ASN: 5607, Name: "Sky UK", Country: "GB", NetworkKind: "residential", Source: "builtin"},
	13285:  {ASN: 13285, Name: "TalkTalk", Country: "GB", NetworkKind: "residential", Source: "builtin"},
	12576:  {ASN: 12576, Name: "EE", Country: "GB", NetworkKind: "mobile", Source: "builtin"},
	5378:   {ASN: 5378, Name: "Vodafone UK", Country: "GB", NetworkKind: "residential", Source: "builtin"},
	3320:   {ASN: 3320, Name: "Deutsche Telekom", Country: "DE", NetworkKind: "residential", Source: "builtin"},
	3209:   {ASN: 3209, Name: "Vodafone Germany", Country: "DE", NetworkKind: "residential", Source: "builtin"},
	6805:   {ASN: 6805, Name: "Telefonica Germany", Country: "DE", NetworkKind: "residential", Source: "builtin"},
	8881:   {ASN: 8881, Name: "1&1 Versatel", Country: "DE", NetworkKind: "residential", Source: "builtin"},
	3215:   {ASN: 3215, Name: "Orange France", Country: "FR", NetworkKind: "residential", Source: "builtin"},
	12322:  {ASN: 12322, Name: "Free SAS", Country: "FR", NetworkKind: "residential", Source: "builtin"},
	15557:  {ASN: 15557, Name: "SFR", Country: "FR", NetworkKind: "residential", Source: "builtin"},
	5410:   {ASN: 5410, Name: "Bouygues Telecom", Country: "FR", NetworkKind: "residential", Source: "builtin"},
	1136:   {ASN: 1136, Name: "KPN", Country: "NL", NetworkKind: "residential", Source: "builtin"},
	9143:   {ASN: 9143, Name: "Ziggo", Country: "NL", NetworkKind: "residential", Source: "builtin"},
	3303:   {ASN: 3303, Name: "Swisscom", Country: "CH", NetworkKind: "residential", Source: "builtin"},
	3301:   {ASN: 3301, Name: "Telia", Country: "SE", NetworkKind: "residential", Source: "builtin"},
	3269:   {ASN: 3269, Name: "TIM", Country: "IT", NetworkKind: "residential", Source: "builtin"},
	12874:  {ASN: 12874, Name: "Fastweb", Country: "IT", NetworkKind: "residential", Source: "builtin"},
	3352:   {ASN: 3352, Name: "Telefonica Spain", Country: "ES", NetworkKind: "residential", Source: "builtin"},
	12479:  {ASN: 12479, Name: "Orange Spain", Country: "ES", NetworkKind: "residential", Source: "builtin"},
	5617:   {ASN: 5617, Name: "Orange Poland", Country: "PL", NetworkKind: "residential", Source: "builtin"},
	4713:   {ASN: 4713, Name: "NTT OCN", Country: "JP", NetworkKind: "residential", Source: "builtin"},
	2516:   {ASN: 2516, Name: "KDDI", Country: "JP", NetworkKind: "residential", Source: "builtin"},
	17676:  {ASN: 17676, Name: "SoftBank", Country: "JP", NetworkKind: "residential", Source: "builtin"},
	2527:   {ASN: 2527, Name: "Sony Network Communications", Country: "JP", NetworkKind: "residential", Source: "builtin"},
	2497:   {ASN: 2497, Name: "IIJ", Country: "JP", NetworkKind: "residential", Source: "builtin"},
	9605:   {ASN: 9605, Name: "NTT DOCOMO", Country: "JP", NetworkKind: "mobile", Source: "builtin"},
	4766:   {ASN: 4766, Name: "KT", Country: "KR", NetworkKind: "residential", Source: "builtin"},
	9318:   {ASN: 9318, Name: "SK Broadband", Country: "KR", NetworkKind: "residential", Source: "builtin"},
	3786:   {ASN: 3786, Name: "LG Uplus", Country: "KR", NetworkKind: "residential", Source: "builtin"},
	9644:   {ASN: 9644, Name: "SK Telecom", Country: "KR", NetworkKind: "mobile", Source: "builtin"},
	3758:   {ASN: 3758, Name: "Singtel", Country: "SG", NetworkKind: "residential", Source: "builtin"},
	4657:   {ASN: 4657, Name: "StarHub", Country: "SG", NetworkKind: "residential", Source: "builtin"},
	4773:   {ASN: 4773, Name: "M1", Country: "SG", NetworkKind: "residential", Source: "builtin"},
	1221:   {ASN: 1221, Name: "Telstra", Country: "AU", NetworkKind: "residential", Source: "builtin"},
	7474:   {ASN: 7474, Name: "Optus", Country: "AU", NetworkKind: "residential", Source: "builtin"},
	7545:   {ASN: 7545, Name: "TPG Telecom", Country: "AU", NetworkKind: "residential", Source: "builtin"},
	4771:   {ASN: 4771, Name: "Spark NZ", Country: "NZ", NetworkKind: "residential", Source: "builtin"},
	9500:   {ASN: 9500, Name: "One NZ", Country: "NZ", NetworkKind: "residential", Source: "builtin"},
	28573:  {ASN: 28573, Name: "Claro Brazil", Country: "BR", NetworkKind: "residential", Source: "builtin"},
	26599:  {ASN: 26599, Name: "Vivo", Country: "BR", NetworkKind: "residential", Source: "builtin"},
	55836:  {ASN: 55836, Name: "Reliance Jio", Country: "IN", NetworkKind: "mobile", Source: "builtin"},
	24560:  {ASN: 24560, Name: "Bharti Airtel", Country: "IN", NetworkKind: "residential", Source: "builtin"},
	9829:   {ASN: 9829, Name: "BSNL", Country: "IN", NetworkKind: "residential", Source: "builtin"},
	4788:   {ASN: 4788, Name: "TM Net", Country: "MY", NetworkKind: "residential", Source: "builtin"},
	7713:   {ASN: 7713, Name: "Telkom Indonesia", Country: "ID", NetworkKind: "residential", Source: "builtin"},
	45899:  {ASN: 45899, Name: "VNPT", Country: "VN", NetworkKind: "residential", Source: "builtin"},
	7552:   {ASN: 7552, Name: "Viettel", Country: "VN", NetworkKind: "residential", Source: "builtin"},
	131445: {ASN: 131445, Name: "AIS Fibre", Country: "TH", NetworkKind: "residential", Source: "builtin"},
	9299:   {ASN: 9299, Name: "PLDT", Country: "PH", NetworkKind: "residential", Source: "builtin"},
}

type AccessASNRepository struct {
	db *sql.DB
}

func NewAccessASNRepository(db *sql.DB) *AccessASNRepository {
	return &AccessASNRepository{db: db}
}

func (r *AccessASNRepository) FindByASN(asn uint64) (*AccessASN, error) {
	if asn == 0 {
		return nil, nil
	}
	if r != nil && r.db != nil {
		var result AccessASN
		err := r.db.QueryRow(
			"SELECT asn, name, country, network_kind, source FROM access_asns WHERE asn = ?",
			asn,
		).Scan(&result.ASN, &result.Name, &result.Country, &result.NetworkKind, &result.Source)
		if err == nil {
			return &result, nil
		}
		if err != sql.ErrNoRows {
			if b, ok := builtinAccessASNMap[asn]; ok {
				cp := b
				return &cp, nil
			}
			return nil, err
		}
	}
	if b, ok := builtinAccessASNMap[asn]; ok {
		cp := b
		return &cp, nil
	}
	return nil, nil
}

func (r *AccessASNRepository) Count() (int64, error) {
	if r == nil || r.db == nil {
		return int64(len(builtinAccessASNMap)), nil
	}
	var count int64
	if err := r.db.QueryRow("SELECT COUNT(*) FROM access_asns").Scan(&count); err != nil {
		return int64(len(builtinAccessASNMap)), nil
	}
	if count == 0 {
		return int64(len(builtinAccessASNMap)), nil
	}
	return count, nil
}
