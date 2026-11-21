import { session } from '../../../services';
import { recordsApi, type Company } from '../../../api/records';
import { RecordList } from '../../../components/RecordList';

const companies = recordsApi(session.api, 'companies');

export default function CompaniesList() {
  return (
    <RecordList<Company>
      title="Companies"
      noun="companies"
      newLabel="New company"
      newHref="/companies/new"
      fetchPage={companies.list}
      row={(company) => ({ name: company.name, subtitle: [company.domain, company.industry].filter(Boolean).join(' · '), href: `/companies/${company.id}` })}
    />
  );
}
