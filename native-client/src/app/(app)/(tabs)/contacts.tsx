import { session } from '../../../services';
import { recordsApi, type Contact } from '../../../api/records';
import { RecordList } from '../../../components/RecordList';
import { contactName, contactSubtitle } from '../../../lib/records/format';
import { t, useLocale } from '../../../i18n';

const contacts = recordsApi(session.api, 'contacts');

export default function ContactsList() {
  useLocale();
  return (
    <RecordList<Contact>
      title={t('Contacts')}
      noun="contacts"
      newLabel={t('New contact')}
      newHref="/contacts/new"
      fetchPage={contacts.list}
      row={(contact) => ({ name: contactName(contact), subtitle: contactSubtitle(contact), href: `/contacts/${contact.id}` })}
    />
  );
}
