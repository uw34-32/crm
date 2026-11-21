import { session } from '../../../services';
import { recordsApi, type Contact } from '../../../api/records';
import { RecordList } from '../../../components/RecordList';
import { contactName, contactSubtitle } from '../../../lib/records/format';

const contacts = recordsApi(session.api, 'contacts');

export default function ContactsList() {
  return (
    <RecordList<Contact>
      title="Contacts"
      noun="contacts"
      newLabel="New contact"
      newHref="/contacts/new"
      fetchPage={contacts.list}
      row={(contact) => ({ name: contactName(contact), subtitle: contactSubtitle(contact), href: `/contacts/${contact.id}` })}
    />
  );
}
