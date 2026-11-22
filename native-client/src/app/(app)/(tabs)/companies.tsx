import { session } from '../../../services';
import { recordsApi, type Company } from '../../../api/records';
import { RecordList } from '../../../components/RecordList';
import { t, useLocale } from '../../../i18n';

const companies = recordsApi(session.api, 'companies');

export default function CompaniesList() {
  useLocale();
  return (
    <RecordList<Company>
      title={t('Companies')}
      noun="companies"
      newLabel={t('New company')}
      newHref="/companies/new"
      fetchPage={companies.list}
      row={(company) => ({ name: company.name, subtitle: [company.domain, company.industry].filter(Boolean).join(' · '), href: `/companies/${company.id}` })}
    />
  );
}
