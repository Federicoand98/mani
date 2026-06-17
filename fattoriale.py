def fattoriale(n):
    """
    Calcola il fattoriale di un numero non negativo.
    
    Args:
        n: un numero intero non negativo
    
    Returns:
        Il fattoriale del numero dato
    
    Raises:
        ValueError: se n è negativo
    """
    if n < 0:
        raise ValueError("Il fattoriale è definito solo per numeri non negativi.")
    
    if n == 0 or n == 1:
        return 1
    
    risultato = 1
    for i in range(2, n + 1):
        risultato *= i
    
    return risultato


def fattoriale_ricorsivo(n):
    """
    Versione ricorsiva della funzione fattoriale.
    
    Args:
        n: un numero intero non negativo
    
    Returns:
        Il fattoriale del numero dato
    
    Raises:
        ValueError: se n è negativo
        RecursionError: se n è troppo grande (limite di ricorsione)
    """
    if n < 0:
        raise ValueError("Il fattoriale è definito solo per numeri non negativi.")
    
    if n == 0 or n == 1:
        return 1
    
    return n * fattoriale_ricorsivo(n - 1)


def main():
    """Funzione principale che gestisce l'input e l'esecuzione."""
    print("=" * 50)
    print("CALCOLATORE DEL FATTORIALE")
    print("=" * 50)
    print()
    
    try:
        input_str = input("Inserisci un numero non negativo: ")
        n = int(input_str)
        risultato = fattoriale(n)
        print()
        print(f"Input: {n}")
        print(f"Fattoriale ({n}!) = {risultato}")
        print()
        
        if risultato > 10**18:
            print(f"Valore approssimato: {risultato:.2e}")
        
        print("=" * 50)
        
    except ValueError as ve:
        print(f"Errore: {ve}")
        print("Inserisci un numero valido!")
    except OverflowError as oe:
        print(f"Errore: Il numero è troppo grande!")
    except Exception as e:
        print(f"Si è verificato un errore: {e}")
    finally:
        print("Grazie per aver usato il calcolatore!")


if __name__ == "__main__":
    main()
