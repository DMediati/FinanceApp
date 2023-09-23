import { useEffect, useState } from 'react'
import { usePlaidLink } from 'react-plaid-link'
import './App.css'

function PlaidSignIn({ setAccountBalancesText }: any) {
    const [linkToken, setLinkToken] = useState<string | null>(null);
    const [username, setUsername] = useState<string>("");

    function GetAccountBalances(accessToken: string) {
         fetch('http://localhost:8090/getAccounts', { method:"POST", body: accessToken }).then((response) => {
            return response.text();
        }).then((responseText) => {
            setAccountBalancesText(responseText);        
        });        
    }

    function onSuccess(publicToken: string) {
        console.log(publicToken);
        fetch('http://localhost:8090/getAccessToken', { method:"POST", body: `{ "publicToken": "${publicToken}", "username": "${username}" }` }).then((response) => {
            return response.text();
        }).then((responseText) => {
            setAccountBalancesText(responseText);        
        });        
    }

    function SignIn() {
        fetch('http://localhost:8090/signIn', { method:"POST", body: username }).then((response) => {
            console.log(response)
            return response.json();
        }).then((data) => {
            if (data.accessToken) {
                GetAccountBalances(data.accessToken);
                console.log(data.accessToken)
            } else {
                setLinkToken(data.linkToken)
                console.log(data.linkToken);        
            }
        });
    }

    const Link = ({ linkToken }: { linkToken: string }) => {
        const config: Parameters<typeof usePlaidLink>[0] = {
            token: linkToken,
            // receivedRedirectUri: window.location.href,
            onSuccess,
        };

        const { open, ready } = usePlaidLink(config);

        useEffect(() => {
            if (ready) {
                open();
            }
        }, [ready, open]);

        return (
            <button onClick={() => open()} disabled={!ready}>
                Retry Account Link
            </button>
        );
    }

    return (
        <>
            { linkToken != null ? (
                <Link linkToken={linkToken}/>
            ) : (
                <>
                    <img src="/financeapp.svg"/>
                    <br/><br/>
                    <input key="username-input" type="text" value={username} placeholder='Username' onChange={evt => setUsername(evt.target.value)} />
                    <br/><br/>
                    <button disabled={username.trim() == ""} onClick={()=> SignIn()}>
                        Sign In
                    </button>
                </>
            )}
        </>
    );
}

function App() {
    const [accountBalancesText, setAccountBalancesText] = useState<string | null>(null);

    return (
        <>
            { accountBalancesText &&
                <pre>{accountBalancesText}</pre> ||
                <PlaidSignIn setAccountBalancesText={setAccountBalancesText}/>
            }
        </>
    )
}

export default App
