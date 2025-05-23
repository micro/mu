service helloworld {}

type SearchRequest {
  query string
  type SearchType
  page_number int32
  result_per_page int32
}

type SearchResponse {
  results string
}

enum SearchType {
  SHALLOW = 0
  DEEP = 1
}

endpoint SearchService {
  rpc Search(SearchRequest) returns (SearchResponse)
}